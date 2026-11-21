package postgres

import (
	"context"
	"embed"
	"io/fs"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"claude-memory-sync/internal/domain"
)

const versionTable = "goose_settings_version"

//go:embed migrations/*.sql
var migrationsFS embed.FS

func newProvider(db *sqlx.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, domain.Error(err, "open embedded settings migrations")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db.DB, fsys, goose.WithTableName(versionTable))
	if err != nil {
		return nil, domain.Error(err, "create settings migration provider")
	}
	return provider, nil
}

func Migrate(ctx context.Context, db *sqlx.DB) error {
	provider, err := newProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return domain.Error(err, "apply settings migrations")
	}
	return nil
}
