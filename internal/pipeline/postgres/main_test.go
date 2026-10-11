package postgres

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"claude-memory-sync/internal/pipeline"
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
		postgres.WithDatabase("pipeline"),
		postgres.WithUsername("pipeline"),
		postgres.WithPassword("pipeline"),
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
	db, err := Open(ctx, url)
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
	_, err := testDB.ExecContext(ctx, "TRUNCATE proposal, rejection RESTART IDENTITY CASCADE")
	require.NoError(t, err)
	return New(testDB)
}

func skillProposal(name string) pipeline.NewProposal {
	return pipeline.NewProposal{
		SourceFlow: "daily-synth",
		SourceRun:  "run-1",
		Kind:       pipeline.KindSkill,
		Name:       name,
		Files:      []pipeline.File{{TargetPath: "global/skills/" + name + "/SKILL.md", Content: "body of " + name}},
	}
}

func ruleProposal(name string) pipeline.NewProposal {
	return pipeline.NewProposal{
		SourceFlow: "taskmine",
		SourceRun:  "run-2",
		Kind:       pipeline.KindRule,
		Name:       name + ".md",
		Files:      []pipeline.File{{TargetPath: "global/rules/" + name + ".md", Content: "rule " + name}},
	}
}

func mustCreate(ctx context.Context, t *testing.T, repo *Repository, p pipeline.NewProposal) pipeline.Proposal {
	t.Helper()
	created, err := repo.Create(ctx, p)
	require.NoError(t, err)
	return created
}

func mustMove(ctx context.Context, t *testing.T, repo *Repository, id string, from, to pipeline.Status) pipeline.Proposal {
	t.Helper()
	moved, err := repo.Transition(ctx, pipeline.Transition{ID: id, From: from, To: to, Actor: "tester"})
	require.NoError(t, err)
	return moved
}

func mustApprove(ctx context.Context, t *testing.T, repo *Repository, p pipeline.NewProposal) pipeline.Proposal {
	t.Helper()
	created := mustCreate(ctx, t, repo, p)
	return mustMove(ctx, t, repo, created.ID, pipeline.StatusPending, pipeline.StatusApproved)
}

type backdateArgs struct {
	ID      string  `db:"id"`
	Seconds float64 `db:"seconds"`
}

func backdateCreated(ctx context.Context, t *testing.T, id string, age time.Duration) {
	t.Helper()
	_, err := testDB.NamedExecContext(ctx,
		"UPDATE proposal SET created_at = now() - make_interval(secs => CAST(:seconds AS double precision)) WHERE id = CAST(:id AS uuid)",
		backdateArgs{ID: id, Seconds: age.Seconds()})
	require.NoError(t, err)
}

func backdateDecided(ctx context.Context, t *testing.T, id string, age time.Duration) {
	t.Helper()
	_, err := testDB.NamedExecContext(ctx,
		"UPDATE proposal SET decided_at = now() - make_interval(secs => CAST(:seconds AS double precision)) WHERE id = CAST(:id AS uuid)",
		backdateArgs{ID: id, Seconds: age.Seconds()})
	require.NoError(t, err)
}
