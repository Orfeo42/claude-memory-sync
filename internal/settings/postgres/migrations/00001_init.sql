-- +goose Up
CREATE TABLE IF NOT EXISTS settings (
    key        text PRIMARY KEY,
    value      text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text NOT NULL
);

CREATE TABLE IF NOT EXISTS setting_versions (
    id         bigserial PRIMARY KEY,
    key        text NOT NULL,
    value      text,
    deleted    boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    created_by text NOT NULL
);

CREATE INDEX IF NOT EXISTS setting_versions_key_id_idx ON setting_versions (key, id DESC);

-- +goose Down
DROP INDEX IF EXISTS setting_versions_key_id_idx;
DROP TABLE IF EXISTS setting_versions;
DROP TABLE IF EXISTS settings;
