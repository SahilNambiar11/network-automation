package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/config"
	"github.com/example/distributed-go-network-controller/backend/internal/db"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

const jobExecutionTimeout = 5 * time.Second
const jobLeaseDuration = jobs.DefaultJobLeaseDuration

type jobProcessorRepository interface {
	ClaimNextPendingJob(ctx context.Context, workerID string) (*jobs.Job, error)
	RetryJob(ctx context.Context, jobID string, errMsg string) error
	CompleteJob(ctx context.Context, jobID string, status string, errMsg string) error
	UpdateDeploymentStatus(ctx context.Context, deploymentID string) error
}

func main() {
	cfg := config.Load()

	log.Printf("worker starting with id %q", cfg.WorkerID)

	ctx := context.Background()
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Printf("worker %q connected to postgres", cfg.WorkerID)

	repository := jobs.NewRepository(database)
	pollInterval := 2 * time.Second
	idlePolls := 0
	for {
		job, err := repository.ClaimNextPendingJobWithLease(ctx, cfg.WorkerID, jobLeaseDuration)
		if err != nil {
			log.Printf("failed to claim pending job: %v", err)
			time.Sleep(pollInterval)
			continue
		}
		if job == nil {
			idlePolls++
			if idlePolls == 1 || idlePolls%30 == 0 {
				log.Println("no pending jobs available")
			}
			time.Sleep(pollInterval)
			continue
		}
		idlePolls = 0
		if err := ProcessJobOnce(ctx, repository, *job, jobExecutionTimeout); err != nil {
			log.Printf("failed to process job %s: %v", job.ID, err)
			continue
		}
	}
}

func ProcessJobOnce(ctx context.Context, repository jobProcessorRepository, job jobs.Job, timeout time.Duration) error {
	if job.Reclaimed {
		previousWorker := ""
		if job.PreviousWorker != nil {
			previousWorker = *job.PreviousWorker
		}
		log.Printf("reclaimed expired job %s previously claimed by %s", job.ID, previousWorker)
	}
	log.Printf("claimed job %s for deployment %s on device %s", job.ID, job.DeploymentID, job.DeviceName)

	executionCtx, cancel := context.WithTimeout(ctx, timeout)
	err := ExecuteMockDeployment(executionCtx, job)
	cancel()

	status, retry, errMsg := jobCompletionDecision(job, err)
	if err == nil {
		log.Printf("deployment succeeded for job %s on device %s", job.ID, job.DeviceName)
	} else if status == jobs.JobStatusTimeout {
		log.Printf("deployment timeout for job %s on device %s: %v", job.ID, job.DeviceName, err)
	} else {
		log.Printf("deployment failed for job %s on device %s: %v", job.ID, job.DeviceName, err)
	}

	if retry {
		if err := repository.RetryJob(ctx, job.ID, errMsg); err != nil {
			return fmt.Errorf("schedule retry for job %s: %w", job.ID, err)
		}
		log.Printf("retry scheduled for job %s after attempt %d/%d: %s", job.ID, job.Attempts, job.MaxAttempts, errMsg)
	} else {
		if err := repository.CompleteJob(ctx, job.ID, status, errMsg); err != nil {
			return fmt.Errorf("update job %s status to %s: %w", job.ID, status, err)
		}
		if status == jobs.JobStatusFailed {
			log.Printf("job %s permanently failed after attempt %d/%d", job.ID, job.Attempts, job.MaxAttempts)
		} else if status == jobs.JobStatusTimeout {
			log.Printf("job %s permanently timed out after attempt %d/%d", job.ID, job.Attempts, job.MaxAttempts)
		}
		log.Printf("updated job %s status to %s", job.ID, status)
	}

	if err := repository.UpdateDeploymentStatus(ctx, job.DeploymentID); err != nil {
		return fmt.Errorf("update deployment %s status: %w", job.DeploymentID, err)
	}
	log.Printf("updated deployment %s status after job %s", job.DeploymentID, job.ID)

	return nil
}

func ExecuteMockDeployment(ctx context.Context, job jobs.Job) error {
	log.Printf("starting mock deployment to %s device %s", job.DeviceType, job.DeviceName)

	deviceName := strings.ToLower(job.DeviceName)
	if strings.Contains(deviceName, "fail") {
		return fmt.Errorf("mock deployment failed for device %s", job.DeviceName)
	}

	duration := 10 * time.Millisecond
	if strings.Contains(deviceName, "timeout") {
		duration = jobExecutionTimeout + time.Second
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func jobCompletionDecision(job jobs.Job, err error) (string, bool, string) {
	if err == nil {
		return jobs.JobStatusSuccess, false, ""
	}

	status := jobs.JobStatusFailed
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		status = jobs.JobStatusTimeout
	}

	errMsg := err.Error()
	if job.Attempts < job.MaxAttempts {
		return status, true, errMsg
	}

	return status, false, errMsg
}
