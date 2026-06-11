CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS deployments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status text NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'partial')),
    raw_config text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    device_name text NOT NULL,
    device_type text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'running', 'success', 'failed', 'timeout')),
    attempts int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 3,
    claimed_by text NULL,
    lease_expires_at timestamptz NULL,
    started_at timestamptz NULL,
    completed_at timestamptz NULL,
    error text NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS agents (
    id text PRIMARY KEY,
    hostname text NOT NULL,
    status text NOT NULL,
    last_heartbeat timestamptz NOT NULL DEFAULT now(),
    active_jobs int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS jobs_deployment_id_idx ON jobs (deployment_id);
CREATE INDEX IF NOT EXISTS jobs_status_idx ON jobs (status);
