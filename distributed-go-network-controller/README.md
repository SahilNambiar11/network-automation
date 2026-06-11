# distributed-go-network-controller

Phase 1 scaffold for a distributed Go infrastructure automation platform.

## Architecture

```text
                       +----------------------+
                       | React + Vite frontend|
                       | localhost:5173       |
                       +----------+-----------+
                                  |
                                  v
                       +----------------------+
                       | Go controller API    |
                       | localhost:8080       |
                       | GET /health          |
                       +----------+-----------+
                                  |
                                  v
                       +----------------------+
                       | PostgreSQL           |
                       | localhost:5432       |
                       +----------+-----------+
                                  ^
                                  |
                       +----------------------+
                       | Go worker agents     |
                       | scalable replicas    |
                       +----------------------+
```

## Services

- `backend/cmd/controller`: Go HTTP API service on port `8080`.
- `backend/cmd/worker`: Go worker process with a local goroutine pool for concurrent job execution.
- `postgres`: PostgreSQL database for future controller state.
- `frontend`: React + Vite app on port `5173`.

## Run

```sh
docker compose up --build
```

Open the frontend at:

```text
http://localhost:5173
```

## Test Controller Health

```sh
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok","service":"controller"}
```

## Validate Network YAML

```sh
curl -X POST --data-binary @examples/valid-network.yaml http://localhost:8080/validate
```

Invalid configs return `200 OK` with structured validation errors. Malformed YAML returns `400 Bad Request`.

## Create A Deployment

```sh
curl -X POST http://localhost:8080/deployments \
  --data-binary @examples/valid-network.yaml
```

Then inspect persisted records:

```sh
curl http://localhost:8080/deployments
curl http://localhost:8080/jobs
```

## Test Expired Job Lease Recovery

To manually simulate a worker crash after a job was claimed, expire a running job lease in Postgres:

```sql
UPDATE jobs
SET status = 'running',
    claimed_by = 'dead-worker',
    lease_expires_at = NOW() - INTERVAL '1 minute'
WHERE id = '<job_id>';
```

Another worker can then reclaim the job on its next poll.

## Scale Workers

```sh
docker compose up --build --scale worker=3
```

## Observability

Prometheus is available at:

```text
http://localhost:9090
```

Grafana is available at:

```text
http://localhost:3000
```

Default Grafana login:

```text
admin/admin
```

Grafana provisions the Prometheus datasource and the Network Controller dashboard automatically at startup. No manual datasource setup or dashboard import is required.

## Configuration

The backend reads these environment variables:

- `DATABASE_URL`
- `SERVICE_PORT`
- `WORKER_ID`
- `WORKER_CONCURRENCY` defaults to `3`

## Next Phases

- [ ] Add database migrations.
- [ ] Implement device inventory models.
- [ ] Add YAML parsing and validation.
- [ ] Build job creation and claiming.
- [ ] Add worker job execution flow.
- [ ] Implement drift detection.
- [ ] Expand dashboard data views.
