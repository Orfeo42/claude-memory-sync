-- +goose Up
CREATE TABLE IF NOT EXISTS proposal (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_flow   text NOT NULL,
    source_run    text NOT NULL,
    kind          text NOT NULL,
    name          text NOT NULL,
    status        text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz,
    decided_by    text,
    claimed_by    text,
    claimed_until timestamptz,
    applied_sha   text,
    error         text,
    CONSTRAINT proposal_kind_chk CHECK (kind IN ('skill','rule','agent','global','project-entry','bug','one-time-task','code-change')),
    CONSTRAINT proposal_status_chk CHECK (status IN ('pending','approved','applying','applied','rejected','expired','failed'))
);

CREATE TABLE IF NOT EXISTS proposal_file (
    proposal_id  uuid NOT NULL REFERENCES proposal(id) ON DELETE CASCADE,
    target_path  text NOT NULL,
    content      text NOT NULL,
    PRIMARY KEY (proposal_id, target_path)
);

CREATE TABLE IF NOT EXISTS proposal_event (
    id           bigserial PRIMARY KEY,
    proposal_id  uuid NOT NULL REFERENCES proposal(id) ON DELETE CASCADE,
    from_status  text,
    to_status    text NOT NULL,
    actor        text NOT NULL,
    note         text,
    at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rejection (
    kind         text NOT NULL,
    name         text NOT NULL,
    reason       text,
    rejected_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, name)
);

CREATE INDEX IF NOT EXISTS proposal_status_created_idx ON proposal (status, created_at);
CREATE INDEX IF NOT EXISTS proposal_status_decided_idx ON proposal (status, decided_at);
CREATE INDEX IF NOT EXISTS proposal_status_claimed_idx ON proposal (status, claimed_until);
CREATE INDEX IF NOT EXISTS proposal_kind_status_idx ON proposal (kind, status);
CREATE INDEX IF NOT EXISTS proposal_file_target_idx ON proposal_file (target_path);
CREATE INDEX IF NOT EXISTS proposal_event_proposal_at_idx ON proposal_event (proposal_id, at);

-- +goose Down
DROP INDEX IF EXISTS proposal_event_proposal_at_idx;
DROP INDEX IF EXISTS proposal_file_target_idx;
DROP INDEX IF EXISTS proposal_kind_status_idx;
DROP INDEX IF EXISTS proposal_status_claimed_idx;
DROP INDEX IF EXISTS proposal_status_decided_idx;
DROP INDEX IF EXISTS proposal_status_created_idx;
DROP TABLE IF EXISTS rejection;
DROP TABLE IF EXISTS proposal_event;
DROP TABLE IF EXISTS proposal_file;
DROP TABLE IF EXISTS proposal;
