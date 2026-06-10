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
		WithArgs(JobStatusPending, JobStatusRunning, "worker-1").
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
