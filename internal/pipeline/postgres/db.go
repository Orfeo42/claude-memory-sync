package postgres

import (
	"context"
	"errors"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"claude-memory-sync/internal/domain"
)

func Open(ctx context.Context, databaseURL string) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", databaseURL)
	if err != nil {
		return nil, domain.Error(err, "open pipeline database")
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(domain.Error(err, "ping pipeline database"), db.Close())
	}
	return db, nil
}
