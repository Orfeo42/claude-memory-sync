package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/jmoiron/sqlx"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/settings"
)

const listSettingsQuery = `
SELECT key, value, updated_at, updated_by
FROM settings
ORDER BY key`

const getSettingQuery = `
SELECT key, value, updated_at, updated_by
FROM settings
WHERE key = :key`

const upsertSettingQuery = `
INSERT INTO settings (key, value, updated_by)
VALUES (:key, :value, :actor)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by
RETURNING key, value, updated_at, updated_by`

const deleteSettingQuery = `
DELETE FROM settings WHERE key = :key RETURNING key`

const insertVersionQuery = `
INSERT INTO setting_versions (key, value, deleted, created_by)
VALUES (:key, :audit_value, :deleted, :actor)`

const historyQuery = `
SELECT value, deleted, created_at, created_by
FROM setting_versions
WHERE key = :key
ORDER BY id DESC
LIMIT :limit`

type Repository struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func finish(tx *sqlx.Tx, err error) error {
	if err == nil {
		if commitErr := tx.Commit(); commitErr != nil {
			return domain.Error(commitErr, "commit transaction")
		}
		return nil
	}
	rollbackErr := tx.Rollback()
	if rollbackErr == nil || errors.Is(rollbackErr, sql.ErrTxDone) {
		return err
	}
	return errors.Join(err, domain.Error(rollbackErr, "rollback transaction"))
}

func (r *Repository) List(ctx context.Context) ([]settings.Setting, error) {
	var rows []settingRow
	if err := r.db.SelectContext(ctx, &rows, listSettingsQuery); err != nil {
		return nil, domain.Error(err, "list settings")
	}
	out := make([]settings.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toSetting())
	}
	return out, nil
}

func (r *Repository) Get(ctx context.Context, key settings.Key) (settings.Setting, error) {
	query, args, err := r.db.BindNamed(getSettingQuery, keyArgs{Key: string(key)})
	if err != nil {
		return settings.Setting{}, domain.Error(err, "bind get setting", slog.String("key", string(key)))
	}
	var row settingRow
	err = r.db.GetContext(ctx, &row, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Setting{}, domain.Error(domain.ErrNotFound, "setting not found", slog.String("key", string(key)))
	}
	if err != nil {
		return settings.Setting{}, domain.Error(err, "get setting", slog.String("key", string(key)))
	}
	return row.toSetting(), nil
}

func auditValue(key settings.Key, value string) *string {
	if key.IsSecret() {
		return nil
	}
	return &value
}

func (r *Repository) Put(ctx context.Context, req settings.PutRequest) (settings.Setting, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return settings.Setting{}, domain.Error(err, "begin transaction")
	}
	saved, err := putTx(ctx, tx, req)
	if err := finish(tx, err); err != nil {
		return settings.Setting{}, err
	}
	return saved, nil
}

func putTx(ctx context.Context, tx *sqlx.Tx, req settings.PutRequest) (settings.Setting, error) {
	args := writeArgs{
		Key:        string(req.Key),
		Value:      req.Value,
		AuditValue: auditValue(req.Key, req.Value),
		Actor:      req.Actor,
	}
	query, bound, err := tx.BindNamed(upsertSettingQuery, args)
	if err != nil {
		return settings.Setting{}, domain.Error(err, "bind upsert setting", slog.String("key", args.Key))
	}
	var row settingRow
	if err := tx.GetContext(ctx, &row, query, bound...); err != nil {
		return settings.Setting{}, domain.Error(err, "upsert setting", slog.String("key", args.Key))
	}
	if _, err := tx.NamedExecContext(ctx, insertVersionQuery, args); err != nil {
		return settings.Setting{}, domain.Error(err, "insert setting version", slog.String("key", args.Key))
	}
	return row.toSetting(), nil
}

func (r *Repository) Delete(ctx context.Context, req settings.DeleteRequest) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return domain.Error(err, "begin transaction")
	}
	return finish(tx, deleteTx(ctx, tx, req))
}

func deleteTx(ctx context.Context, tx *sqlx.Tx, req settings.DeleteRequest) error {
	args := writeArgs{Key: string(req.Key), Deleted: true, Actor: req.Actor}
	query, bound, err := tx.BindNamed(deleteSettingQuery, args)
	if err != nil {
		return domain.Error(err, "bind delete setting", slog.String("key", args.Key))
	}
	var deleted string
	err = tx.GetContext(ctx, &deleted, query, bound...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return domain.Error(err, "delete setting", slog.String("key", args.Key))
	}
	if _, err := tx.NamedExecContext(ctx, insertVersionQuery, args); err != nil {
		return domain.Error(err, "insert setting version", slog.String("key", args.Key))
	}
	return nil
}

func (r *Repository) History(ctx context.Context, key settings.Key, limit int) ([]settings.Version, error) {
	query, args, err := r.db.BindNamed(historyQuery, historyArgs{Key: string(key), Limit: limit})
	if err != nil {
		return nil, domain.Error(err, "bind setting history", slog.String("key", string(key)))
	}
	var rows []versionRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, domain.Error(err, "list setting history", slog.String("key", string(key)))
	}
	out := make([]settings.Version, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toVersion())
	}
	return out, nil
}
