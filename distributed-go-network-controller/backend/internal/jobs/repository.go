package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

const (
	DeploymentStatusPending = "pending"
	JobStatusPending        = "pending"
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
		var job Job
		var claimedBy sql.NullString
		var leaseExpiresAt sql.NullTime
		var startedAt sql.NullTime
		var completedAt sql.NullTime
		var jobError sql.NullString

		if err := rows.Scan(
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
			return nil, fmt.Errorf("scan job: %w", err)
		}

		job.ClaimedBy = nullableString(claimedBy)
		job.LeaseExpiresAt = nullableTime(leaseExpiresAt)
		job.StartedAt = nullableTime(startedAt)
		job.CompletedAt = nullableTime(completedAt)
		job.Error = nullableString(jobError)
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}

	return jobs, nil
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
