package main

import (
	"context"
	"errors"
	"strings"
	"sync"
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

func TestRunWorkerPoolProcessesMultipleJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := newFakeJobProcessorRepository(
		fakeJob("job-1", "core-router", "router", 3),
		fakeJob("job-2", "access-switch", "switch", 3),
		fakeJob("job-3", "edge-router", "router", 3),
		fakeJob("job-4", "branch-switch", "switch", 3),
	)
	repository.cancelWhenCompleted = cancel
	repository.completionTarget = 4

	RunWorkerPool(ctx, repository, "worker-1", 2, &activeJobCounter{})

	for _, jobID := range []string{"job-1", "job-2", "job-3", "job-4"} {
		job := repository.job(jobID)
		if job.Status != jobs.JobStatusSuccess {
			t.Fatalf("expected %s status success, got %q", jobID, job.Status)
		}
		if job.Attempts != 1 {
			t.Fatalf("expected %s attempts 1, got %d", jobID, job.Attempts)
		}
	}
}

func TestClaimLoopRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repository := newFakeJobProcessorRepository()
	jobsCh := make(chan jobs.Job, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		claimLoop(ctx, repository, "worker-1", jobsCh, time.Hour, jobLeaseDuration)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected claim loop to stop after context cancellation")
	}
}

func TestExecutorStopsWhenJobsChannelCloses(t *testing.T) {
	ctx := context.Background()
	repository := newFakeJobProcessorRepository()
	jobsCh := make(chan jobs.Job)
	var wg sync.WaitGroup

	wg.Add(1)
	go executorLoop(ctx, 1, repository, jobsCh, &activeJobCounter{}, &wg)
	close(jobsCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected executor to stop when jobs channel closes")
	}
}

func TestActiveJobCounterUpdates(t *testing.T) {
	counter := &activeJobCounter{}

	counter.Increment()
	counter.Increment()
	if counter.Load() != 2 {
		t.Fatalf("expected active jobs 2, got %d", counter.Load())
	}

	counter.Decrement()
	if counter.Load() != 1 {
		t.Fatalf("expected active jobs 1, got %d", counter.Load())
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
	mu                  sync.Mutex
	jobs                map[string]jobs.Job
	completed           int
	completionTarget    int
	cancelWhenCompleted context.CancelFunc
}

func newFakeJobProcessorRepository(jobsList ...jobs.Job) *fakeJobProcessorRepository {
	repository := &fakeJobProcessorRepository{jobs: make(map[string]jobs.Job)}
	for _, job := range jobsList {
		repository.jobs[job.ID] = job
	}
	return repository
}

func (r *fakeJobProcessorRepository) ClaimNextPendingJob(ctx context.Context, workerID string) (*jobs.Job, error) {
	return r.ClaimNextPendingJobWithLease(ctx, workerID, jobLeaseDuration)
}

func (r *fakeJobProcessorRepository) WorkerTablesReady(ctx context.Context) error {
	return nil
}

func (r *fakeJobProcessorRepository) UpsertAgentHeartbeat(ctx context.Context, agentID, hostname string, activeJobs int) error {
	return nil
}

func (r *fakeJobProcessorRepository) ClaimNextPendingJobWithLease(ctx context.Context, workerID string, leaseDuration time.Duration) (*jobs.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.completed++
	if r.cancelWhenCompleted != nil && r.completionTarget > 0 && r.completed >= r.completionTarget {
		r.cancelWhenCompleted()
	}
	return nil
}

func (r *fakeJobProcessorRepository) UpdateDeploymentStatus(ctx context.Context, deploymentID string) error {
	return nil
}

func (r *fakeJobProcessorRepository) job(jobID string) jobs.Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.jobs[jobID]
}
