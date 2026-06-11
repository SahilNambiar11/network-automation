package metrics

import (
	"context"
	"time"

	"github.com/example/distributed-go-network-controller/backend/internal/drift"
	"github.com/example/distributed-go-network-controller/backend/internal/jobs"
	"github.com/prometheus/client_golang/prometheus"
)

const scrapeTimeout = 10 * time.Second

type Repository interface {
	GetDeployments(ctx context.Context) ([]jobs.Deployment, error)
	GetJobs(ctx context.Context) ([]jobs.Job, error)
	ListAgents(ctx context.Context) ([]jobs.Agent, error)
	GenerateAllDriftReports(ctx context.Context) ([]drift.DriftReport, error)
}

type Collector struct {
	repository Repository

	deploymentsTotal       *prometheus.Desc
	jobsTotal              *prometheus.Desc
	jobsSuccessTotal       *prometheus.Desc
	jobsFailedTotal        *prometheus.Desc
	jobsTimeoutTotal       *prometheus.Desc
	jobsRunningTotal       *prometheus.Desc
	jobsPendingTotal       *prometheus.Desc
	activeAgents           *prometheus.Desc
	unhealthyAgents        *prometheus.Desc
	devicesCheckedForDrift *prometheus.Desc
	devicesWithDrift       *prometheus.Desc
	workerActiveJobs       *prometheus.Desc
}

func NewCollector(repository Repository) *Collector {
	return &Collector{
		repository: repository,

		deploymentsTotal:       prometheus.NewDesc("deployments_total", "Current number of deployments.", nil, nil),
		jobsTotal:              prometheus.NewDesc("jobs_total", "Current number of jobs.", nil, nil),
		jobsSuccessTotal:       prometheus.NewDesc("jobs_success_total", "Current number of successful jobs.", nil, nil),
		jobsFailedTotal:        prometheus.NewDesc("jobs_failed_total", "Current number of failed jobs.", nil, nil),
		jobsTimeoutTotal:       prometheus.NewDesc("jobs_timeout_total", "Current number of timed out jobs.", nil, nil),
		jobsRunningTotal:       prometheus.NewDesc("jobs_running_total", "Current number of running jobs.", nil, nil),
		jobsPendingTotal:       prometheus.NewDesc("jobs_pending_total", "Current number of pending jobs.", nil, nil),
		activeAgents:           prometheus.NewDesc("active_agents", "Current number of healthy agents.", nil, nil),
		unhealthyAgents:        prometheus.NewDesc("unhealthy_agents", "Current number of unhealthy agents.", nil, nil),
		devicesCheckedForDrift: prometheus.NewDesc("devices_checked_for_drift", "Current number of devices checked for drift.", nil, nil),
		devicesWithDrift:       prometheus.NewDesc("devices_with_drift", "Current number of devices with drift.", nil, nil),
		workerActiveJobs:       prometheus.NewDesc("worker_active_jobs", "Current total active jobs reported by workers.", nil, nil),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.deploymentsTotal
	ch <- c.jobsTotal
	ch <- c.jobsSuccessTotal
	ch <- c.jobsFailedTotal
	ch <- c.jobsTimeoutTotal
	ch <- c.jobsRunningTotal
	ch <- c.jobsPendingTotal
	ch <- c.activeAgents
	ch <- c.unhealthyAgents
	ch <- c.devicesCheckedForDrift
	ch <- c.devicesWithDrift
	ch <- c.workerActiveJobs
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()

	snapshot, err := c.snapshot(ctx)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.deploymentsTotal, err)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.deploymentsTotal, prometheus.GaugeValue, float64(snapshot.deploymentsTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsTotal, prometheus.GaugeValue, float64(snapshot.jobsTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsSuccessTotal, prometheus.GaugeValue, float64(snapshot.jobsSuccessTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsFailedTotal, prometheus.GaugeValue, float64(snapshot.jobsFailedTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsTimeoutTotal, prometheus.GaugeValue, float64(snapshot.jobsTimeoutTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsRunningTotal, prometheus.GaugeValue, float64(snapshot.jobsRunningTotal))
	ch <- prometheus.MustNewConstMetric(c.jobsPendingTotal, prometheus.GaugeValue, float64(snapshot.jobsPendingTotal))
	ch <- prometheus.MustNewConstMetric(c.activeAgents, prometheus.GaugeValue, float64(snapshot.activeAgents))
	ch <- prometheus.MustNewConstMetric(c.unhealthyAgents, prometheus.GaugeValue, float64(snapshot.unhealthyAgents))
	ch <- prometheus.MustNewConstMetric(c.devicesCheckedForDrift, prometheus.GaugeValue, float64(snapshot.devicesCheckedForDrift))
	ch <- prometheus.MustNewConstMetric(c.devicesWithDrift, prometheus.GaugeValue, float64(snapshot.devicesWithDrift))
	ch <- prometheus.MustNewConstMetric(c.workerActiveJobs, prometheus.GaugeValue, float64(snapshot.workerActiveJobs))
}

type snapshot struct {
	deploymentsTotal       int
	jobsTotal              int
	jobsSuccessTotal       int
	jobsFailedTotal        int
	jobsTimeoutTotal       int
	jobsRunningTotal       int
	jobsPendingTotal       int
	activeAgents           int
	unhealthyAgents        int
	devicesCheckedForDrift int
	devicesWithDrift       int
	workerActiveJobs       int
}

func (c *Collector) snapshot(ctx context.Context) (snapshot, error) {
	deployments, err := c.repository.GetDeployments(ctx)
	if err != nil {
		return snapshot{}, err
	}

	currentJobs, err := c.repository.GetJobs(ctx)
	if err != nil {
		return snapshot{}, err
	}

	agents, err := c.repository.ListAgents(ctx)
	if err != nil {
		return snapshot{}, err
	}

	driftReports, err := c.repository.GenerateAllDriftReports(ctx)
	if err != nil {
		return snapshot{}, err
	}

	result := snapshot{
		deploymentsTotal:       len(deployments),
		jobsTotal:              len(currentJobs),
		devicesCheckedForDrift: len(driftReports),
	}

	for _, currentJob := range currentJobs {
		switch currentJob.Status {
		case jobs.JobStatusSuccess:
			result.jobsSuccessTotal++
		case jobs.JobStatusFailed:
			result.jobsFailedTotal++
		case jobs.JobStatusTimeout:
			result.jobsTimeoutTotal++
		case jobs.JobStatusRunning:
			result.jobsRunningTotal++
		case jobs.JobStatusPending:
			result.jobsPendingTotal++
		}
	}

	for _, agent := range jobs.AgentsWithComputedHealth(agents, time.Now()) {
		result.workerActiveJobs += agent.ActiveJobs
		if agent.Status == jobs.AgentStatusUnhealthy {
			result.unhealthyAgents++
			continue
		}

		result.activeAgents++
	}

	for _, report := range driftReports {
		if report.Drift {
			result.devicesWithDrift++
		}
	}

	return result, nil
}
