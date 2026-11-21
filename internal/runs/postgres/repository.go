package postgres

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/runs"
)

const (
	invalidTextRepresentation = "22P02"
	interruptedMessage        = "interrupted: synth restarted"
)

const runColumns = `CAST(id AS text) AS id, flow, trigger, status, started_at, finished_at, error, summary`

const stepColumns = `id, CAST(run_id AS text) AS run_id, step, status, started_at, finished_at, error, output_tail`

const insertRunQuery = `
INSERT INTO runs (flow, trigger, status)
VALUES (:flow, :trigger, 'running')
RETURNING ` + runColumns

const insertStepQuery = `
INSERT INTO run_steps (run_id, step, status, started_at, finished_at, error, output_tail)
SELECT r.id, CAST(:step AS text), CAST(:status AS text), CAST(:started_at AS timestamptz),
    CAST(:finished_at AS timestamptz), CAST(:error AS text), CAST(:output_tail AS text)
FROM runs r
WHERE r.id = CAST(:run_id AS uuid) AND r.status = 'running'
RETURNING ` + stepColumns

const finishRunQuery = `
UPDATE runs SET
    status = CAST(:status AS text),
    finished_at = now(),
    error = CAST(:error AS text),
    summary = COALESCE(CAST(:summary AS jsonb), summary)
WHERE id = CAST(:id AS uuid) AND status = 'running'
RETURNING ` + runColumns

const runStatusQuery = `SELECT status FROM runs WHERE id = CAST(:id AS uuid)`

const interruptRunsQuery = `
UPDATE runs SET
    status = 'interrupted',
    finished_at = now(),
    error = CAST(:message AS text)
WHERE status = 'running'
    AND (CAST(:flow AS text) IS NULL OR flow = CAST(:flow AS text))`

const listRunsQuery = `
SELECT ` + runColumns + `
FROM runs
WHERE (CAST(:flow AS text) IS NULL OR flow = CAST(:flow AS text))
    AND (CAST(:status AS text) IS NULL OR status = CAST(:status AS text))
    AND (CAST(:before AS timestamptz) IS NULL OR started_at < CAST(:before AS timestamptz))
ORDER BY started_at DESC, id
LIMIT :limit`

const getRunQuery = `SELECT ` + runColumns + ` FROM runs WHERE id = CAST(:id AS uuid)`

const listStepsQuery = `
SELECT ` + stepColumns + `
FROM run_steps
WHERE run_id = CAST(:id AS uuid)
ORDER BY id`

const pruneRunsQuery = `
DELETE FROM runs
WHERE status <> 'running'
    AND started_at < now() - make_interval(secs => CAST(:seconds AS double precision))`

type Repository struct {
	db *sqlx.DB
}

var _ runs.Repository = (*Repository)(nil)

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func dbError(err error, msg string, attrs ...slog.Attr) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation {
		return domain.Error(domain.ErrNotFound, "run not found", attrs...)
	}
	return domain.Error(err, msg, attrs...)
}

func (r *Repository) CreateRun(ctx context.Context, n runs.NewRun) (runs.Run, error) {
	query, args, err := r.db.BindNamed(insertRunQuery, createArgs{Flow: n.Flow, Trigger: string(n.Trigger)})
	if err != nil {
		return runs.Run{}, dbError(err, "bind create run")
	}
	var rows []runRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return runs.Run{}, dbError(err, "create run", slog.String("flow", n.Flow))
	}
	if len(rows) == 0 {
		return runs.Run{}, domain.Error(domain.ErrInternal, "create run returned no row", slog.String("flow", n.Flow))
	}
	return rows[0].toRun(), nil
}

func (r *Repository) missingOrConflict(ctx context.Context, id string) error {
	query, args, err := r.db.BindNamed(runStatusQuery, idArgs{ID: id})
	if err != nil {
		return dbError(err, "bind run status", slog.String("run_id", id))
	}
	var rows []statusRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return dbError(err, "read run status", slog.String("run_id", id))
	}
	if len(rows) == 0 {
		return domain.Error(domain.ErrNotFound, "run not found", slog.String("run_id", id))
	}
	return domain.Error(domain.ErrConflict, "run is not running",
		slog.String("run_id", id),
		slog.String("run_status", rows[0].Status),
	)
}

func (r *Repository) AddStep(ctx context.Context, s runs.NewStep) (runs.Step, error) {
	query, args, err := r.db.BindNamed(insertStepQuery, stepArgs{
		RunID:      s.RunID,
		Step:       s.Step,
		Status:     string(s.Status),
		StartedAt:  s.StartedAt,
		FinishedAt: s.FinishedAt,
		Error:      s.Error,
		OutputTail: s.OutputTail,
	})
	if err != nil {
		return runs.Step{}, dbError(err, "bind add step", slog.String("run_id", s.RunID))
	}
	var rows []stepRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return runs.Step{}, dbError(err, "add step", slog.String("run_id", s.RunID))
	}
	if len(rows) == 0 {
		return runs.Step{}, r.missingOrConflict(ctx, s.RunID)
	}
	return rows[0].toStep(), nil
}

func (r *Repository) FinishRun(ctx context.Context, f runs.Finish) (runs.Run, error) {
	var summary *string
	if len(f.Summary) > 0 {
		text := string(f.Summary)
		summary = &text
	}

	query, args, err := r.db.BindNamed(finishRunQuery, finishArgs{
		ID:      f.ID,
		Status:  string(f.Status),
		Error:   f.Error,
		Summary: summary,
	})
	if err != nil {
		return runs.Run{}, dbError(err, "bind finish run", slog.String("run_id", f.ID))
	}
	var rows []runRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return runs.Run{}, dbError(err, "finish run", slog.String("run_id", f.ID))
	}
	if len(rows) == 0 {
		return runs.Run{}, r.missingOrConflict(ctx, f.ID)
	}
	return rows[0].toRun(), nil
}

func (r *Repository) InterruptRuns(ctx context.Context, flow *string) (int, error) {
	res, err := r.db.NamedExecContext(ctx, interruptRunsQuery, interruptArgs{Flow: flow, Message: interruptedMessage})
	if err != nil {
		return 0, dbError(err, "interrupt runs")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, dbError(err, "count interrupted runs")
	}
	return int(n), nil
}

func (r *Repository) ListRuns(ctx context.Context, f runs.ListFilter) ([]runs.Run, error) {
	args := listArgs{Flow: f.Flow, Before: f.Before, Limit: f.Limit}
	if f.Status != nil {
		status := string(*f.Status)
		args.Status = &status
	}

	query, bound, err := r.db.BindNamed(listRunsQuery, args)
	if err != nil {
		return nil, dbError(err, "bind list runs")
	}
	var rows []runRow
	if err := r.db.SelectContext(ctx, &rows, query, bound...); err != nil {
		return nil, dbError(err, "list runs")
	}

	list := make([]runs.Run, 0, len(rows))
	for _, row := range rows {
		list = append(list, row.toRun())
	}
	return list, nil
}

func (r *Repository) GetRun(ctx context.Context, id string) (runs.Run, error) {
	query, args, err := r.db.BindNamed(getRunQuery, idArgs{ID: id})
	if err != nil {
		return runs.Run{}, dbError(err, "bind get run", slog.String("run_id", id))
	}
	var rows []runRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return runs.Run{}, dbError(err, "get run", slog.String("run_id", id))
	}
	if len(rows) == 0 {
		return runs.Run{}, domain.Error(domain.ErrNotFound, "run not found", slog.String("run_id", id))
	}
	return rows[0].toRun(), nil
}

func (r *Repository) Steps(ctx context.Context, runID string) ([]runs.Step, error) {
	query, args, err := r.db.BindNamed(listStepsQuery, idArgs{ID: runID})
	if err != nil {
		return nil, dbError(err, "bind list steps", slog.String("run_id", runID))
	}
	var rows []stepRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, dbError(err, "list steps", slog.String("run_id", runID))
	}

	steps := make([]runs.Step, 0, len(rows))
	for _, row := range rows {
		steps = append(steps, row.toStep())
	}
	return steps, nil
}

func (r *Repository) PruneRuns(ctx context.Context, olderThan time.Duration) (int, error) {
	res, err := r.db.NamedExecContext(ctx, pruneRunsQuery, pruneArgs{Seconds: olderThan.Seconds()})
	if err != nil {
		return 0, dbError(err, "prune runs")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, dbError(err, "count pruned runs")
	}
	return int(n), nil
}
