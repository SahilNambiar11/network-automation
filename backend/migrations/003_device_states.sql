CREATE TABLE IF NOT EXISTS device_states (
    device_name text PRIMARY KEY,
    device_type text NOT NULL,
    actual_config jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
