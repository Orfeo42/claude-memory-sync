package postgres

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}
	os.Exit(runWithContainer(m))
}

func runWithContainer(m *testing.M) int {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("settings"),
		postgres.WithUsername("settings"),
		postgres.WithPassword("settings"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		return 1
	}

	code := runWithDB(ctx, m, container)
	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", err)
	}
	return code
}

func runWithDB(ctx context.Context, m *testing.M, container *postgres.PostgresContainer) int {
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		return 1
	}
	db, err := sqlx.ConnectContext(ctx, "pgx", url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		return 1
	}
	testDB = db

	if err := Migrate(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 1
	}
	code := m.Run()
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close database: %v\n", err)
	}
	return code
}

func setup(ctx context.Context, t *testing.T) *Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs postgres")
	}
	_, err := testDB.ExecContext(ctx, "TRUNCATE settings, setting_versions RESTART IDENTITY")
	require.NoError(t, err)
	return New(testDB)
}
