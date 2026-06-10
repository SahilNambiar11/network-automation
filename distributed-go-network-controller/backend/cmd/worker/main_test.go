package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

func TestProcessJobOnceSuccessfulDeployment(t *testing.T) {
	repository := newFakeJobProcessorRepository(fakeJob("job-1", "core-router", "router", 3))

	if err := processNextJobForTest(context.Background(), repository, 100*time.Millisecond); err != nil {
		t.Fatalf("process job: %v", err)
	}

	job := repository.jobs["job-1"]
	if job.Status != jobs.JobStatusSuccess {
		t.Fatalf("expected status success, got %q", job.Status)
	}
	if job.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", job.Attempts)
	}
	if job.CompletedAt == nil {
		t.Fatalf("expected completed_at to be populated")
	}
	if job.Error != nil {
		t.Fatalf("expected empty error, got %q", *job.Error)
	}
}

func TestProcessJobOncePermanentFailure(t *testing.T) {
	repository := newFakeJobProcessorRepository(fakeJob("job-1", "fail-switch", "switch", 3))

	for i := 0; i < 3; i++ {
		if err := processNextJobForTest(context.Background(), repository, 100*time.Millisecond); err != nil {
			t.Fatalf("process job attempt %d: %v", i+1, err)
		}
	}

	job := repository.jobs["job-1"]
	if job.Attempts != 3 {
		t.Fatalf("expected attempts 3, got %d", job.Attempts)
	}
	if job.Status != jobs.JobStatusFailed {
		t.Fatalf("expected final status failed, got %q", job.Status)
	}
	if job.Error == nil || !strings.Contains(*job.Error, "mock deployment failed") {
		t.Fatalf("expected failure error, got %#v", job.Error)
	}
	if job.CompletedAt == nil {
		t.Fatalf("expected completed_at to be populated")
	}
}

func TestProcessJobOncePermanentTimeout(t *testing.T) {
	repository := newFakeJobProcessorRepository(fakeJob("job-1", "timeout-firewall", "firewall", 3))

	for i := 0; i < 3; i++ {
		if err := processNextJobForTest(context.Background(), repository, 100*time.Millisecond); err != nil {
			t.Fatalf("process job attempt %d: %v", i+1, err)
		}
	}

	job := repository.jobs["job-1"]
	if job.Attempts != 3 {
		t.Fatalf("expected attempts 3, got %d", job.Attempts)
	}
	if job.Status != jobs.JobStatusTimeout {
		t.Fatalf("expected final status timeout, got %q", job.Status)
	}
	if job.Error == nil || !strings.Contains(*job.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected timeout error, got %#v", job.Error)
	}
	if job.CompletedAt == nil {
		t.Fatalf("expected completed_at to be populated")
	}
}

func TestProcessJobOnceRetryBeforeMaxAttempts(t *testing.T) {
	repository := newFakeJobProcessorRepository(fakeJob("job-1", "fail-switch", "switch", 3))

	if err := processNextJobForTest(context.Background(), repository, 100*time.Millisecond); err != nil {
		t.Fatalf("process job: %v", err)
	}

	job := repository.jobs["job-1"]
	if job.Attempts != 1 {
		t.Fatalf("expected attempts 1, got %d", job.Attempts)
	}
	if job.Status != jobs.JobStatusPending {
		t.Fatalf("expected status pending, got %q", job.Status)
	}
	if job.ClaimedBy != nil {
		t.Fatalf("expected claimed_by to be empty, got %q", *job.ClaimedBy)
	}
	if job.CompletedAt != nil {
		t.Fatalf("expected completed_at to be empty")
	}
	if job.Error == nil || *job.Error == "" {
		t.Fatalf("expected populated error")
	}
}

func TestExecuteMockDeploymentFailure(t *testing.T) {
	job := jobs.Job{DeviceName: "fail-switch", DeviceType: "switch"}

	err := ExecuteMockDeployment(context.Background(), job)
	if err == nil {
		t.Fatalf("expected failure")
	}
}

func TestExecuteMockDeploymentTimeout(t *testing.T) {
	job := jobs.Job{DeviceName: "timeout-firewall", DeviceType: "firewall"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := ExecuteMockDeployment(ctx, job)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func processNextJobForTest(ctx context.Context, repository *fakeJobProcessorRepository, timeout time.Duration) error {
	job, err := repository.ClaimNextPendingJob(ctx, "worker-1")
	if err != nil {
		return err
	}
	if job == nil {
		return errors.New("expected pending job")
	}

	return ProcessJobOnce(ctx, repository, *job, timeout)
}

func fakeJob(id string, deviceName string, deviceType string, maxAttempts int) jobs.Job {
	return jobs.Job{
		ID:           id,
		DeploymentID: "deployment-1",
		DeviceName:   deviceName,
		DeviceType:   deviceType,
		Status:       jobs.JobStatusPending,
		MaxAttempts:  maxAttempts,
		CreatedAt:    time.Now(),
	}
}

type fakeJobProcessorRepository struct {
	jobs map[string]jobs.Job
}

func newFakeJobProcessorRepository(jobsList ...jobs.Job) *fakeJobProcessorRepository {
	repository := &fakeJobProcessorRepository{jobs: make(map[string]jobs.Job)}
	for _, job := range jobsList {
		repository.jobs[job.ID] = job
	}
	return repository
}

func (r *fakeJobProcessorRepository) ClaimNextPendingJob(ctx context.Context, workerID string) (*jobs.Job, error) {
	for _, job := range r.jobs {
		if job.Status != jobs.JobStatusPending {
			continue
		}

		now := time.Now()
		claimedBy := workerID
		job.Status = jobs.JobStatusRunning
		job.ClaimedBy = &claimedBy
		job.StartedAt = &now
		job.Attempts++
		r.jobs[job.ID] = job
		return &job, nil
	}

	return nil, nil
}

func (r *fakeJobProcessorRepository) RetryJob(ctx context.Context, jobID string, errMsg string) error {
	job := r.jobs[jobID]
	job.Status = jobs.JobStatusPending
	job.ClaimedBy = nil
	job.LeaseExpiresAt = nil
	job.CompletedAt = nil
	job.Error = &errMsg
	r.jobs[jobID] = job
	return nil
}

func (r *fakeJobProcessorRepository) CompleteJob(ctx context.Context, jobID string, status string, errMsg string) error {
	job := r.jobs[jobID]
	now := time.Now()
	job.Status = status
	job.CompletedAt = &now
	if errMsg == "" {
		job.Error = nil
	} else {
		job.Error = &errMsg
	}
	r.jobs[jobID] = job
	return nil
}

func (r *fakeJobProcessorRepository) UpdateDeploymentStatus(ctx context.Context, deploymentID string) error {
	return nil
}
