package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

const (
	DefaultJobLeaseDuration = 30 * time.Second

	DeploymentStatusPending = "pending"
	DeploymentStatusRunning = "running"
	DeploymentStatusSuccess = "success"
	DeploymentStatusFailed  = "failed"
	DeploymentStatusPartial = "partial"
	JobStatusPending        = "pending"
	JobStatusRunning        = "running"
	JobStatusSuccess        = "success"
	JobStatusFailed         = "failed"
	JobStatusTimeout        = "timeout"
)

type Repository struct {
	db *sql.DB
}

type Deployment struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	RawConfig   string     `json:"raw_config"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Job struct {
	ID             string     `json:"id"`
	DeploymentID   string     `json:"deployment_id"`
	DeviceName     string     `json:"device_name"`
	DeviceType     string     `json:"device_type"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	ClaimedBy      *string    `json:"claimed_by,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Error          *string    `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Reclaimed      bool       `json:"-"`
	PreviousWorker *string    `json:"-"`
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateDeployment(ctx context.Context, rawConfig string) (string, error) {
	var deploymentID string
	if err := r.db.QueryRowContext(ctx, `
		INSERT INTO deployments (status, raw_config)
		VALUES ($1, $2)
		RETURNING id
	`, DeploymentStatusPending, rawConfig).Scan(&deploymentID); err != nil {
		return "", fmt.Errorf("create deployment: %w", err)
	}

	return deploymentID, nil
}

func (r *Repository) CreateJobsForDeployment(ctx context.Context, deploymentID string, devicesList []devices.Device) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create jobs transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO jobs (deployment_id, device_name, device_type, status)
		VALUES ($1, $2, $3, $4)
	`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare create jobs statement: %w", err)
	}
	defer stmt.Close()

	for _, device := range devicesList {
		if _, err := stmt.ExecContext(ctx, deploymentID, device.Name, device.Type, JobStatusPending); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("create job for device %s: %w", device.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create jobs transaction: %w", err)
	}

	return nil
}

func (r *Repository) ClaimNextPendingJob(ctx context.Context, workerID string) (*Job, error) {
	return r.ClaimNextPendingJobWithLease(ctx, workerID, DefaultJobLeaseDuration)
}

func (r *Repository) ClaimNextPendingJobWithLease(ctx context.Context, workerID string, leaseDuration time.Duration) (*Job, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim job transaction: %w", err)
	}

	leaseSeconds := leaseDuration.Seconds()
	row := tx.QueryRowContext(ctx, `
		WITH next_job AS (
			SELECT id, status, claimed_by
			FROM jobs
			WHERE status = $1
				OR (status = $2 AND lease_expires_at < now())
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE jobs
		SET status = $2,
			claimed_by = $3,
			started_at = now(),
			lease_expires_at = now() + ($4::double precision * INTERVAL '1 second'),
			attempts = attempts + 1
		FROM next_job
		WHERE jobs.id = next_job.id
		RETURNING jobs.id, jobs.deployment_id, jobs.device_name, jobs.device_type, jobs.status,
			jobs.attempts, jobs.max_attempts, jobs.claimed_by, jobs.lease_expires_at,
			jobs.started_at, jobs.completed_at, jobs.error, jobs.created_at,
			next_job.status, next_job.claimed_by
	`, JobStatusPending, JobStatusRunning, workerID, leaseSeconds)

	job, err := scanClaimedJob(row)
	if err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim next pending job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim job transaction: %w", err)
	}

	return &job, nil
}

func (r *Repository) CompleteJob(ctx context.Context, jobID string, status string, errMsg string) error {
	if status != JobStatusSuccess && status != JobStatusFailed && status != JobStatusTimeout {
		return fmt.Errorf("unsupported job completion status %q", status)
	}

	var errorValue any
	if status == JobStatusFailed || status == JobStatusTimeout {
		errorValue = errMsg
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $1,
			completed_at = now(),
			error = $2
		WHERE id = $3
	`, status, errorValue, jobID); err != nil {
		return fmt.Errorf("complete job %s: %w", jobID, err)
	}

	return nil
}

func (r *Repository) RetryJob(ctx context.Context, jobID string, errMsg string) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $1,
			claimed_by = NULL,
			lease_expires_at = NULL,
			completed_at = NULL,
			error = $2
		WHERE id = $3
	`, JobStatusPending, errMsg, jobID); err != nil {
		return fmt.Errorf("retry job %s: %w", jobID, err)
	}

	return nil
}

func (r *Repository) UpdateDeploymentStatus(ctx context.Context, deploymentID string) error {
	var totalJobs int
	var successJobs int
	var failedJobs int
	var runningJobs int
	var pendingJobs int

	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = $1),
			COUNT(*) FILTER (WHERE status IN ($2, $3)),
			COUNT(*) FILTER (WHERE status = $4),
			COUNT(*) FILTER (WHERE status = $5)
		FROM jobs
		WHERE deployment_id = $6
	`, JobStatusSuccess, JobStatusFailed, JobStatusTimeout, JobStatusRunning, JobStatusPending, deploymentID).Scan(
		&totalJobs,
		&successJobs,
		&failedJobs,
		&runningJobs,
		&pendingJobs,
	); err != nil {
		return fmt.Errorf("count jobs for deployment %s: %w", deploymentID, err)
	}

	status, completed := deploymentStatusFromCounts(totalJobs, successJobs, failedJobs, runningJobs, pendingJobs)

	if completed {
		if _, err := r.db.ExecContext(ctx, `
			UPDATE deployments
			SET status = $1,
				completed_at = now()
			WHERE id = $2
		`, status, deploymentID); err != nil {
			return fmt.Errorf("update completed deployment %s status: %w", deploymentID, err)
		}
		return nil
	}

	if _, err := r.db.ExecContext(ctx, `
		UPDATE deployments
		SET status = $1,
			completed_at = NULL
		WHERE id = $2
	`, status, deploymentID); err != nil {
		return fmt.Errorf("update deployment %s status: %w", deploymentID, err)
	}

	return nil
}

func deploymentStatusFromCounts(totalJobs, successJobs, failedJobs, runningJobs, pendingJobs int) (string, bool) {
	switch {
	case pendingJobs > 0 || runningJobs > 0:
		return DeploymentStatusRunning, false
	case totalJobs > 0 && successJobs == totalJobs:
		return DeploymentStatusSuccess, true
	case totalJobs > 0 && failedJobs > 0:
		if failedJobs == totalJobs {
			return DeploymentStatusFailed, true
		}
		return DeploymentStatusPartial, true
	default:
		return DeploymentStatusPending, false
	}
}

func canClaimJob(status string, leaseExpiresAt *time.Time, now time.Time) bool {
	if status == JobStatusPending {
		return true
	}
	if status != JobStatusRunning || leaseExpiresAt == nil {
		return false
	}

	return leaseExpiresAt.Before(now)
}

func (r *Repository) GetDeployments(ctx context.Context) ([]Deployment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, status, raw_config, created_at, completed_at
		FROM deployments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("get deployments: %w", err)
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployments: %w", err)
	}

	return deployments, nil
}

func (r *Repository) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, status, raw_config, created_at, completed_at
		FROM deployments
		WHERE id = $1
	`, id)

	deployment, err := scanDeployment(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &deployment, nil
}

func (r *Repository) GetJobs(ctx context.Context) ([]Job, error) {
	rows, err := r.db.QueryContext(ctx, jobsSelectQuery+`
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("get jobs: %w", err)
	}
	defer rows.Close()

	return scanJobs(rows)
}

func (r *Repository) GetJobsByDeployment(ctx context.Context, deploymentID string) ([]Job, error) {
	rows, err := r.db.QueryContext(ctx, jobsSelectQuery+`
		WHERE deployment_id = $1
		ORDER BY created_at DESC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("get jobs by deployment: %w", err)
	}
	defer rows.Close()

	return scanJobs(rows)
}

const jobsSelectQuery = `
	SELECT id, deployment_id, device_name, device_type, status, attempts, max_attempts,
		claimed_by, lease_expires_at, started_at, completed_at, error, created_at
	FROM jobs
`

type deploymentScanner interface {
	Scan(dest ...any) error
}

func scanDeployment(scanner deploymentScanner) (Deployment, error) {
	var deployment Deployment
	var completedAt sql.NullTime

	if err := scanner.Scan(
		&deployment.ID,
		&deployment.Status,
		&deployment.RawConfig,
		&deployment.CreatedAt,
		&completedAt,
	); err != nil {
		return Deployment{}, fmt.Errorf("scan deployment: %w", err)
	}

	deployment.CompletedAt = nullableTime(completedAt)
	return deployment, nil
}

func scanJobs(rows *sql.Rows) ([]Job, error) {
	var jobs []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}

	return jobs, nil
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(scanner jobScanner) (Job, error) {
	var job Job
	var claimedBy sql.NullString
	var leaseExpiresAt sql.NullTime
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var jobError sql.NullString

	if err := scanner.Scan(
		&job.ID,
		&job.DeploymentID,
		&job.DeviceName,
		&job.DeviceType,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&claimedBy,
		&leaseExpiresAt,
		&startedAt,
		&completedAt,
		&jobError,
		&job.CreatedAt,
	); err != nil {
		return Job{}, err
	}

	job.ClaimedBy = nullableString(claimedBy)
	job.LeaseExpiresAt = nullableTime(leaseExpiresAt)
	job.StartedAt = nullableTime(startedAt)
	job.CompletedAt = nullableTime(completedAt)
	job.Error = nullableString(jobError)

	return job, nil
}

func scanClaimedJob(scanner jobScanner) (Job, error) {
	var job Job
	var claimedBy sql.NullString
	var leaseExpiresAt sql.NullTime
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var jobError sql.NullString
	var previousStatus string
	var previousWorker sql.NullString

	if err := scanner.Scan(
		&job.ID,
		&job.DeploymentID,
		&job.DeviceName,
		&job.DeviceType,
		&job.Status,
		&job.Attempts,
		&job.MaxAttempts,
		&claimedBy,
		&leaseExpiresAt,
		&startedAt,
		&completedAt,
		&jobError,
		&job.CreatedAt,
		&previousStatus,
		&previousWorker,
	); err != nil {
		return Job{}, err
	}

	job.ClaimedBy = nullableString(claimedBy)
	job.LeaseExpiresAt = nullableTime(leaseExpiresAt)
	job.StartedAt = nullableTime(startedAt)
	job.CompletedAt = nullableTime(completedAt)
	job.Error = nullableString(jobError)
	job.Reclaimed = previousStatus == JobStatusRunning
	job.PreviousWorker = nullableString(previousWorker)

	return job, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}

	return &value.Time
}
