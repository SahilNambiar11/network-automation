package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/config"
	"github.com/example/distributed-go-network-controller/backend/internal/db"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
)

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
		job, err := repository.ClaimNextPendingJob(ctx, cfg.WorkerID)
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

		log.Printf("claimed job %s for deployment %s on device %s", job.ID, job.DeploymentID, job.DeviceName)

		status := jobs.JobStatusSuccess
		errMsg := ""
		if err := ExecuteMockDeployment(ctx, *job); err != nil {
			status = jobs.JobStatusFailed
			errMsg = err.Error()
			log.Printf("deployment failed for job %s on device %s: %v", job.ID, job.DeviceName, err)
		} else {
			log.Printf("deployment succeeded for job %s on device %s", job.ID, job.DeviceName)
		}

		if err := repository.CompleteJob(ctx, job.ID, status, errMsg); err != nil {
			log.Printf("failed to update job %s status to %s: %v", job.ID, status, err)
			continue
		}
		log.Printf("updated job %s status to %s", job.ID, status)

		if err := repository.UpdateDeploymentStatus(ctx, job.DeploymentID); err != nil {
			log.Printf("failed to update deployment %s status: %v", job.DeploymentID, err)
			continue
		}
		log.Printf("updated deployment %s status after job %s", job.DeploymentID, job.ID)
	}
}

func ExecuteMockDeployment(ctx context.Context, job jobs.Job) error {
	log.Printf("starting mock deployment to %s device %s", job.DeviceType, job.DeviceName)

	duration := time.Duration(rand.Intn(3)+1) * time.Second
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
