# Kubernetes Deployment

This directory makes Kubernetes a first-class runtime option without changing the controller's distributed coordination model.

Docker Compose remains the simplest local development path. Kubernetes is intended for production-style orchestration: running containers, restarting crashed pods, service discovery, rolling deploys, config/secrets, probes, and worker replica scaling.

## Architecture Boundary

Kubernetes owns infrastructure:

- controller, worker, frontend, Prometheus, Grafana, and demo PostgreSQL pods
- container restarts and rolling updates
- services and in-cluster networking
- ConfigMaps and Secrets
- liveness, readiness, and startup probes
- worker pod replica count

The Go application still owns correctness:

- PostgreSQL job queue
- `FOR UPDATE SKIP LOCKED` job claiming
- job leases and lease expiration
- retries and timeouts
- worker heartbeats
- deployment lifecycle
- device state
- desired vs actual drift detection
- Prometheus application metrics

Kubernetes does not schedule per-device jobs. Worker pods safely compete for jobs through PostgreSQL.

## Images

Build and publish or load these images into your cluster:

```sh
docker build -t distributed-go-network-controller/controller:latest -f backend/Dockerfile.controller backend
docker build -t distributed-go-network-controller/worker:latest -f backend/Dockerfile.worker backend
docker build -t distributed-go-network-controller/frontend:latest -f frontend/Dockerfile frontend
```

For a local cluster such as kind or minikube, load the images into that cluster before applying the manifests.

## Deploy

```sh
kubectl apply -k deploy/kubernetes
```

Check rollout status:

```sh
kubectl -n network-controller rollout status deployment/postgres
kubectl -n network-controller rollout status deployment/controller
kubectl -n network-controller rollout status deployment/worker
kubectl -n network-controller rollout status deployment/frontend
```

Port-forward locally:

```sh
kubectl -n network-controller port-forward svc/frontend 5173:5173
kubectl -n network-controller port-forward svc/controller 8080:8080
kubectl -n network-controller port-forward svc/prometheus 9090:9090
kubectl -n network-controller port-forward svc/grafana 3000:3000
```

## Scaling Workers

Scale worker pods without changing controller logic:

```sh
kubectl -n network-controller scale deployment/worker --replicas=5
```

Each worker gets a stable `WORKER_ID` from its pod name and claims jobs with the existing PostgreSQL lease protocol.

## Verification

After port-forwarding the controller, create a multi-device deployment:

```sh
curl http://localhost:8080/health
curl -X POST --data-binary @examples/many-devices.yaml http://localhost:8080/deployments
curl http://localhost:8080/jobs
curl http://localhost:8080/agents
```

A healthy run should show:

- deployment creation returning `jobs_created`
- jobs moving to `success`
- multiple worker pod names in `claimed_by`
- healthy agents with recent heartbeats

Prometheus verification:

```sh
kubectl -n network-controller port-forward svc/prometheus 9090:9090
```

Then query:

- `up`
- `deployments_total`
- `jobs_total`
- `jobs_success_total`
- `jobs_pending_total`
- `active_agents`
- `worker_active_jobs`
- `devices_with_drift`

## Resource Choices

The manifests use conservative demo defaults:

- controller and workers request `100m` CPU and `128Mi` memory, with `500m` / `256Mi` limits.
- PostgreSQL, Prometheus, and Grafana request `100m` CPU and `256Mi` memory, with `500m` / `512Mi` limits.

These are small enough for local clusters but explicit enough to exercise Kubernetes scheduling and avoid unbounded memory use. For real production, tune these from observed Prometheus data.

## Production Notes

The included PostgreSQL deployment is for demos. A realistic infrastructure product should usually use a managed PostgreSQL service or a dedicated database platform with backups, upgrades, monitoring, and restore testing.

Replace the demo Secret values before using these manifests outside a local environment.
