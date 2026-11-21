-- +goose Up
CREATE TABLE IF NOT EXISTS runs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    flow        text NOT NULL,
    trigger     text NOT NULL,
    status      text NOT NULL,
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    error       text,
    summary     jsonb NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT runs_trigger_chk CHECK (trigger IN ('schedule','manual','event')),
    CONSTRAINT runs_status_chk CHECK (status IN ('running','succeeded','failed','partial','interrupted'))
);

CREATE TABLE IF NOT EXISTS run_steps (
    id          bigserial PRIMARY KEY,
    run_id      uuid NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    step        text NOT NULL,
    status      text NOT NULL,
    started_at  timestamptz NOT NULL,
    finished_at timestamptz NOT NULL,
    error       text,
    output_tail text,
    CONSTRAINT run_steps_status_chk CHECK (status IN ('succeeded','failed','skipped','lossy'))
);

CREATE INDEX IF NOT EXISTS runs_flow_started_idx ON runs (flow, started_at DESC);
CREATE INDEX IF NOT EXISTS runs_started_idx ON runs (started_at DESC);
CREATE INDEX IF NOT EXISTS run_steps_run_idx ON run_steps (run_id, id);

-- +goose Down
DROP INDEX IF EXISTS run_steps_run_idx;
DROP INDEX IF EXISTS runs_started_idx;
DROP INDEX IF EXISTS runs_flow_started_idx;
DROP TABLE IF EXISTS run_steps;
DROP TABLE IF EXISTS runs;
