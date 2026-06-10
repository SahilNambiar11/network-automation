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
- `backend/cmd/worker`: Go worker process that logs its worker ID and heartbeat.
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

## Scale Workers

```sh
docker compose up --build --scale worker=3
```

## Configuration

The backend reads these environment variables:

- `DATABASE_URL`
- `SERVICE_PORT`
- `WORKER_ID`

## Next Phases

- [ ] Add database migrations.
- [ ] Implement device inventory models.
- [ ] Add YAML parsing and validation.
- [ ] Build job creation and claiming.
- [ ] Add worker job execution flow.
- [ ] Implement drift detection.
- [ ] Expand dashboard data views.
