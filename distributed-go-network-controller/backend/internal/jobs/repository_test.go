package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/example/distributed-go-network-controller/backend/internal/devices"
)

func TestRepositoryCreateDeployment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	rawConfig := "devices:\n  - name: core-router\n    type: router\n"

	mock.ExpectQuery("INSERT INTO deployments").
		WithArgs(DeploymentStatusPending, rawConfig).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("deployment-1"))

	deploymentID, err := repository.CreateDeployment(context.Background(), rawConfig)
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	if deploymentID != "deployment-1" {
		t.Fatalf("expected deployment id deployment-1, got %q", deploymentID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryCreateJobsForDeployment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	devicesList := []devices.Device{
		{Name: "core-router", Type: "router"},
		{Name: "access-switch", Type: "switch"},
	}

	mock.ExpectBegin()
	mock.ExpectPrepare("INSERT INTO jobs").
		ExpectExec().
		WithArgs("deployment-1", "core-router", "router", JobStatusPending).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO jobs").
		WithArgs("deployment-1", "access-switch", "switch", JobStatusPending).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.CreateJobsForDeployment(context.Background(), "deployment-1", devicesList); err != nil {
		t.Fatalf("create jobs for deployment: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryClaimNextPendingJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery("WITH next_job AS").
		WithArgs(JobStatusPending, JobStatusRunning, "worker-1", DefaultJobLeaseDuration.Seconds()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"deployment_id",
			"device_name",
			"device_type",
			"status",
			"attempts",
			"max_attempts",
			"claimed_by",
			"lease_expires_at",
			"started_at",
			"completed_at",
			"error",
			"created_at",
			"previous_status",
			"previous_claimed_by",
		}).AddRow(
			"job-1",
			"deployment-1",
			"core-router",
			"router",
			JobStatusRunning,
			1,
			3,
			sql.NullString{String: "worker-1", Valid: true},
			sql.NullTime{},
			sql.NullTime{Time: now, Valid: true},
			sql.NullTime{},
			sql.NullString{},
			now,
			JobStatusPending,
			sql.NullString{},
		))
	mock.ExpectCommit()

	job, err := repository.ClaimNextPendingJob(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("claim next pending job: %v", err)
	}
	if job == nil {
		t.Fatalf("expected claimed job")
	}
	if job.ID != "job-1" {
		t.Fatalf("expected job id job-1, got %q", job.ID)
	}
	if job.Status != JobStatusRunning {
		t.Fatalf("expected status running, got %q", job.Status)
	}
	if job.ClaimedBy == nil || *job.ClaimedBy != "worker-1" {
		t.Fatalf("expected claimed_by worker-1, got %#v", job.ClaimedBy)
	}
	if job.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", job.Attempts)
	}
	if job.StartedAt == nil {
		t.Fatalf("expected started_at to be populated")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryClaimExpiredRunningJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	now := time.Now()
	leaseExpiresAt := now.Add(30 * time.Second)

	mock.ExpectBegin()
	mock.ExpectQuery("WITH next_job AS").
		WithArgs(JobStatusPending, JobStatusRunning, "worker-b", 10.0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"deployment_id",
			"device_name",
			"device_type",
			"status",
			"attempts",
			"max_attempts",
			"claimed_by",
			"lease_expires_at",
			"started_at",
			"completed_at",
			"error",
			"created_at",
			"previous_status",
			"previous_claimed_by",
		}).AddRow(
			"job-1",
			"deployment-1",
			"core-router",
			"router",
			JobStatusRunning,
			2,
			3,
			sql.NullString{String: "worker-b", Valid: true},
			sql.NullTime{Time: leaseExpiresAt, Valid: true},
			sql.NullTime{Time: now, Valid: true},
			sql.NullTime{},
			sql.NullString{},
			now,
			JobStatusRunning,
			sql.NullString{String: "worker-a", Valid: true},
		))
	mock.ExpectCommit()

	job, err := repository.ClaimNextPendingJobWithLease(context.Background(), "worker-b", 10*time.Second)
	if err != nil {
		t.Fatalf("claim expired running job: %v", err)
	}
	if job == nil {
		t.Fatalf("expected claimed job")
	}
	if job.ClaimedBy == nil || *job.ClaimedBy != "worker-b" {
		t.Fatalf("expected claimed_by worker-b, got %#v", job.ClaimedBy)
	}
	if !job.Reclaimed {
		t.Fatalf("expected job to be marked reclaimed")
	}
	if job.PreviousWorker == nil || *job.PreviousWorker != "worker-a" {
		t.Fatalf("expected previous worker worker-a, got %#v", job.PreviousWorker)
	}
	if job.Attempts != 2 {
		t.Fatalf("expected attempts 2, got %d", job.Attempts)
	}
	if job.LeaseExpiresAt == nil {
		t.Fatalf("expected lease_expires_at to be populated")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCanClaimJobByStatusAndLease(t *testing.T) {
	now := time.Now()
	expiredLease := now.Add(-time.Minute)
	unexpiredLease := now.Add(time.Minute)

	tests := []struct {
		name           string
		status         string
		leaseExpiresAt *time.Time
		want           bool
	}{
		{name: "pending job can be claimed", status: JobStatusPending, want: true},
		{name: "running job with unexpired lease cannot be claimed", status: JobStatusRunning, leaseExpiresAt: &unexpiredLease, want: false},
		{name: "running job with expired lease can be reclaimed", status: JobStatusRunning, leaseExpiresAt: &expiredLease, want: true},
		{name: "success job cannot be reclaimed", status: JobStatusSuccess, leaseExpiresAt: &expiredLease, want: false},
		{name: "failed job cannot be reclaimed", status: JobStatusFailed, leaseExpiresAt: &expiredLease, want: false},
		{name: "timeout job cannot be reclaimed", status: JobStatusTimeout, leaseExpiresAt: &expiredLease, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canClaimJob(tt.status, tt.leaseExpiresAt, now)
			if got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestRepositoryCompleteJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectExec("UPDATE jobs").
		WithArgs(JobStatusSuccess, nil, "job-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.CompleteJob(context.Background(), "job-1", JobStatusSuccess, ""); err != nil {
		t.Fatalf("complete job: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryCompleteJobTimeout(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectExec("UPDATE jobs").
		WithArgs(JobStatusTimeout, "deployment timed out", "job-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.CompleteJob(context.Background(), "job-1", JobStatusTimeout, "deployment timed out"); err != nil {
		t.Fatalf("complete timeout job: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryRetryJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectExec("UPDATE jobs").
		WithArgs(JobStatusPending, "temporary failure", "job-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.RetryJob(context.Background(), "job-1", "temporary failure"); err != nil {
		t.Fatalf("retry job: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryUpsertAgentHeartbeatInsertsNewAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectExec("INSERT INTO agents").
		WithArgs("worker-1", "host-a", AgentStatusHealthy, 2).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.UpsertAgentHeartbeat(context.Background(), "worker-1", "host-a", 2); err != nil {
		t.Fatalf("upsert agent heartbeat: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryUpsertAgentHeartbeatUpdatesExistingAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectExec("INSERT INTO agents").
		WithArgs("worker-1", "host-b", AgentStatusHealthy, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.UpsertAgentHeartbeat(context.Background(), "worker-1", "host-b", 1); err != nil {
		t.Fatalf("upsert existing agent heartbeat: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryListAgents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	now := time.Now()

	mock.ExpectQuery("SELECT id, hostname, status, last_heartbeat, active_jobs, created_at").
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"hostname",
			"status",
			"last_heartbeat",
			"active_jobs",
			"created_at",
		}).AddRow("worker-1", "host-a", AgentStatusHealthy, now, 2, now))

	agents, err := repository.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ID != "worker-1" {
		t.Fatalf("expected worker-1, got %q", agents[0].ID)
	}
	if agents[0].ActiveJobs != 2 {
		t.Fatalf("expected active jobs 2, got %d", agents[0].ActiveJobs)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryGetDeviceState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	now := time.Now()
	actualConfig := []byte(`{"vlans":[{"id":10}],"firewall_rules":[]}`)

	mock.ExpectQuery("SELECT device_name, device_type, actual_config, updated_at").
		WithArgs("core-router").
		WillReturnRows(sqlmock.NewRows([]string{
			"device_name",
			"device_type",
			"actual_config",
			"updated_at",
		}).AddRow("core-router", "router", actualConfig, now))

	state, err := repository.GetDeviceState(context.Background(), "core-router")
	if err != nil {
		t.Fatalf("get device state: %v", err)
	}
	if state == nil {
		t.Fatalf("expected device state")
	}
	if state.DeviceName != "core-router" {
		t.Fatalf("expected core-router, got %q", state.DeviceName)
	}
	if string(state.ActualConfig) != string(actualConfig) {
		t.Fatalf("expected actual config %s, got %s", actualConfig, state.ActualConfig)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryListDeviceStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)
	now := time.Now()

	mock.ExpectQuery("SELECT device_name, device_type, actual_config, updated_at").
		WillReturnRows(sqlmock.NewRows([]string{
			"device_name",
			"device_type",
			"actual_config",
			"updated_at",
		}).
			AddRow("access-switch", "switch", []byte(`{"vlans":[]}`), now).
			AddRow("core-router", "router", []byte(`{"vlans":[{"id":10}]}`), now))

	states, err := repository.ListDeviceStates(context.Background())
	if err != nil {
		t.Fatalf("list device states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 device states, got %d", len(states))
	}
	if states[0].DeviceName != "access-switch" {
		t.Fatalf("expected access-switch first, got %q", states[0].DeviceName)
	}
	if states[1].DeviceName != "core-router" {
		t.Fatalf("expected core-router second, got %q", states[1].DeviceName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestAgentStatusAtMarksStaleHeartbeatUnhealthy(t *testing.T) {
	now := time.Now()
	status := AgentStatusAt(now.Add(-AgentHeartbeatTimeout-time.Second), now)

	if status != AgentStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %q", status)
	}
}

func TestAgentsWithComputedHealthKeepsFreshHeartbeatHealthy(t *testing.T) {
	now := time.Now()
	agents := AgentsWithComputedHealth([]Agent{
		{ID: "worker-1", Status: AgentStatusUnhealthy, LastHeartbeat: now.Add(-time.Second)},
	}, now)

	if agents[0].Status != AgentStatusHealthy {
		t.Fatalf("expected healthy, got %q", agents[0].Status)
	}
}

func TestRepositoryUpdateDeploymentStatusSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectQuery("SELECT").
		WithArgs(JobStatusSuccess, JobStatusFailed, JobStatusTimeout, JobStatusRunning, JobStatusPending, "deployment-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"count",
			"success_count",
			"failed_count",
			"running_count",
			"pending_count",
		}).AddRow(2, 2, 0, 0, 0))
	mock.ExpectExec("UPDATE deployments").
		WithArgs(DeploymentStatusSuccess, "deployment-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.UpdateDeploymentStatus(context.Background(), "deployment-1"); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryUpdateDeploymentStatusPartial(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectQuery("SELECT").
		WithArgs(JobStatusSuccess, JobStatusFailed, JobStatusTimeout, JobStatusRunning, JobStatusPending, "deployment-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"count",
			"success_count",
			"failed_count",
			"running_count",
			"pending_count",
		}).AddRow(3, 1, 2, 0, 0))
	mock.ExpectExec("UPDATE deployments").
		WithArgs(DeploymentStatusPartial, "deployment-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.UpdateDeploymentStatus(context.Background(), "deployment-1"); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRepositoryUpdateDeploymentStatusFailed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer db.Close()

	repository := NewRepository(db)

	mock.ExpectQuery("SELECT").
		WithArgs(JobStatusSuccess, JobStatusFailed, JobStatusTimeout, JobStatusRunning, JobStatusPending, "deployment-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"count",
			"success_count",
			"failed_count",
			"running_count",
			"pending_count",
		}).AddRow(2, 0, 2, 0, 0))
	mock.ExpectExec("UPDATE deployments").
		WithArgs(DeploymentStatusFailed, "deployment-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repository.UpdateDeploymentStatus(context.Background(), "deployment-1"); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDeploymentStatusFromCountsSuccess(t *testing.T) {
	status, completed := deploymentStatusFromCounts(2, 2, 0, 0, 0)

	if status != DeploymentStatusSuccess {
		t.Fatalf("expected status success, got %q", status)
	}
	if !completed {
		t.Fatalf("expected deployment to be completed")
	}
}

func TestDeploymentStatusFromCountsPartial(t *testing.T) {
	status, completed := deploymentStatusFromCounts(2, 1, 1, 0, 0)

	if status != DeploymentStatusPartial {
		t.Fatalf("expected status partial, got %q", status)
	}
	if !completed {
		t.Fatalf("expected deployment to be completed")
	}
}

func TestDeploymentStatusFromCountsRunning(t *testing.T) {
	status, completed := deploymentStatusFromCounts(2, 1, 0, 0, 1)

	if status != DeploymentStatusRunning {
		t.Fatalf("expected status running, got %q", status)
	}
	if completed {
		t.Fatalf("expected deployment to remain incomplete")
	}
}
