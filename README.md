# Distributed Go Network Automation Controller

A Go-based infrastructure automation/control-plane project that validates desired network configuration, creates deployments and jobs, and executes them with distributed worker agents. It tracks mock device state for simulated routers, switches, and firewalls, detects configuration drift, and exposes observability through Prometheus, Grafana, and a React dashboard.

The project is designed as a demoable distributed systems exercise: PostgreSQL-backed coordination, worker leases, retries, timeouts, heartbeats, drift reports, metrics, dashboards, and a browser UI all run locally from Docker Compose. Kubernetes is also supported as an optional production-style runtime layer for container orchestration, health checks, rolling deploys, service discovery, and worker replica scaling.

## Features

- YAML-based desired network configuration
- VLAN, subnet, and firewall rule validation
- Deployment and job persistence in PostgreSQL
- Distributed worker agents
- PostgreSQL-backed job claiming using `FOR UPDATE SKIP LOCKED`
- Retries and context timeouts
- Lease-based job recovery
- Local goroutine worker pool inside each worker process
- Agent heartbeats and active job tracking
- Simulated device state storage
- Drift detection between desired state and actual mock device state
- Prometheus metrics
- Grafana dashboard
- React frontend dashboard
- Docker Compose local environment
- Optional Kubernetes deployment with scalable worker pods

## Architecture

```text
                         +--------------------------+
                         | React Frontend Dashboard |
                         | http://localhost:5173    |
                         +------------+-------------+
                                      |
                                      v
                         +--------------------------+
                         | Go Controller API        |
                         | http://localhost:8080    |
                         | /health /deployments     |
                         | /jobs /devices /drift    |
                         | /metrics                 |
                         +------+-----------+-------+
                                |           |
                                v           v
                         +-------------+  Drift detection
                         | PostgreSQL  |  desired vs actual
                         | deployments |
                         | jobs        |
                         | agents      |
                         | device_states
                         +------+------+ 
                                ^
                                |
               +----------------+----------------+
               |                                 |
   +-----------+------------+        +-----------+------------+
   | Go Worker Agent        |        | Go Worker Agent        |
   | goroutine worker pool  |        | goroutine worker pool  |
   | job leases + heartbeat |        | job leases + heartbeat |
   +------------------------+        +------------------------+

                         +--------------------------+
                         | Prometheus               |
                         | scrapes controller       |
                         | http://localhost:9090    |
                         +------------+-------------+
                                      |
                                      v
                         +--------------------------+
                         | Grafana Dashboard        |
                         | http://localhost:3000    |
                         +--------------------------+
```

## Core Concepts

- **Desired state**: The submitted YAML network configuration containing devices, VLANs, subnets, and firewall rules.
- **Actual state**: The mock device state stored in PostgreSQL after successful worker deployments.
- **Deployment**: A persisted desired configuration submission.
- **Job**: A per-device unit of work created from a deployment.
- **Controller**: The Go API service that validates configs, creates deployments/jobs, exposes state, generates drift reports, and serves metrics.
- **Worker**: A Go agent process that claims jobs, executes mock deployments, updates device state, and sends heartbeats.
- **Lease**: A database-backed claim window that allows stuck running jobs to be recovered by another worker.
- **Heartbeat**: Periodic worker status and active job reporting stored in PostgreSQL.
- **Drift detection**: Comparison of desired deployment config against actual mock device state.

## API Endpoints

- `GET /health`
- `POST /validate`
- `POST /deployments`
- `GET /deployments`
- `GET /jobs`
- `GET /agents`
- `GET /devices`
- `GET /devices/{name}`
- `POST /devices/{name}/mutate`
- `GET /drift`
- `GET /drift/{device}`
- `GET /drift/summary`
- `GET /metrics`

## Run

```sh
docker compose down -v
WORKER_CONCURRENCY=3 docker compose up --build -d --scale worker=2
```

Open:

- React dashboard: `http://localhost:5173`
- Controller API: `http://localhost:8080`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`

Grafana default login:

```text
admin/admin
```

## Kubernetes

Kubernetes manifests live in `deploy/kubernetes`. They are an optional production-style deployment path; Docker Compose remains the easiest local development workflow.

The Kubernetes deployment intentionally preserves the existing distributed systems design. Worker pods can be scaled as replicas, but per-device job scheduling, `FOR UPDATE SKIP LOCKED` claiming, leases, retries, heartbeats, drift detection, and deployment lifecycle remain inside the Go application and PostgreSQL.

Kubernetes is used for:

- Running the controller, worker, frontend, PostgreSQL, Prometheus, and Grafana containers
- Restarting crashed pods
- Service discovery and in-cluster networking
- ConfigMaps and Secrets
- Liveness, readiness, and startup probes
- Scaling worker pod replicas

Kubernetes is not used as the job scheduler. The workers still compete for application jobs through PostgreSQL.

Build local images:

```sh
docker build -t distributed-go-network-controller/controller:latest -f backend/Dockerfile.controller backend
docker build -t distributed-go-network-controller/worker:latest -f backend/Dockerfile.worker backend
docker build -t distributed-go-network-controller/frontend:latest -f frontend/Dockerfile frontend
```

Apply the manifests:

```sh
kubectl apply -k deploy/kubernetes
kubectl -n network-controller get pods
```

Scale workers:

```sh
kubectl -n network-controller scale deployment/worker --replicas=5
```

Port-forward the controller and verify the system:

```sh
kubectl -n network-controller port-forward svc/controller 8080:8080
curl http://localhost:8080/health
curl -X POST --data-binary @examples/many-devices.yaml http://localhost:8080/deployments
curl http://localhost:8080/jobs
curl http://localhost:8080/agents
```

Prometheus can be checked with:

```sh
kubectl -n network-controller port-forward svc/prometheus 9090:9090
```

Useful Prometheus queries:

- `up`
- `deployments_total`
- `jobs_total`
- `jobs_success_total`
- `jobs_pending_total`
- `active_agents`
- `worker_active_jobs`
- `devices_with_drift`

See `deploy/kubernetes/README.md` for image, rollout, port-forwarding, and production notes.

## Demo Workflow

Validate YAML:

```sh
curl -X POST --data-binary @examples/valid-network.yaml http://localhost:8080/validate
```

Create a deployment:

```sh
curl -X POST --data-binary @examples/valid-network.yaml http://localhost:8080/deployments
```

Check jobs:

```sh
curl http://localhost:8080/jobs
```

Check devices:

```sh
curl http://localhost:8080/devices
```

Check drift:

```sh
curl http://localhost:8080/drift
```

Mutate device state to create drift:

```sh
curl -X POST http://localhost:8080/devices/core-router/mutate \
  -H "Content-Type: application/json" \
  -d '{"remove_vlan":10}'
```

Check drift again:

```sh
curl http://localhost:8080/drift/core-router
```

Check metrics:

```sh
curl http://localhost:8080/metrics | grep -E "deployments_total|jobs_total|devices_with_drift|active_agents"
```

## Observability

The controller exposes Prometheus metrics at `GET /metrics`. Prometheus scrapes the controller at `controller:8080/metrics`, and Grafana is automatically provisioned with the Prometheus datasource and Network Controller dashboard.

Metrics include deployments, total jobs, job statuses, active agents, unhealthy agents, worker active jobs, devices checked for drift, and devices with drift.

## Frontend

The React dashboard supports:

- Validating and deploying YAML configuration
- Viewing deployments and jobs
- Viewing worker agents and heartbeat status
- Viewing mock device state
- Mutating device state for drift demos
- Viewing drift reports and drift summary counts
- Opening Prometheus and Grafana

The frontend runs at `http://localhost:5173` and proxies API requests to the controller through Vite.

## Testing

Backend:

```sh
cd backend && go test ./...
```

Frontend:

```sh
cd frontend && npm run build
```

Docker:

```sh
docker compose build
```

## Configuration

- `DATABASE_URL`: PostgreSQL connection string for controller and workers.
- `SERVICE_PORT`: Controller HTTP port, defaulted by Compose to `8080`.
- `WORKER_ID`: Optional worker identifier. If omitted, workers generate one from the environment/container.
- `WORKER_CONCURRENCY`: Number of goroutine executors inside each worker process. Defaults to `3`.
- `VITE_PROXY_TARGET`: Frontend development proxy target. Compose sets this to `http://controller:8080`.

## Project Status

Completed:

- Validation
- Persistence
- Workers
- Retries and timeouts
- Lease recovery
- Worker pool
- Heartbeats
- Device state
- Drift detection
- Prometheus and Grafana
- React dashboard
- Optional Kubernetes deployment
- Graceful worker shutdown for pod termination

Future improvements:

- Automatic drift remediation/reconciliation loop
- GitHub Actions CI
- Horizontal Pod Autoscaling from queue depth or custom Prometheus metrics
- Real network device adapter interface
