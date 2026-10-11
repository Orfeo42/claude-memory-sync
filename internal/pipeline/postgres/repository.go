package postgres

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
)

const invalidTextRepresentation = "22P02"

const proposalSelect = `
SELECT CAST(p.id AS text) AS id, p.source_flow, p.source_run, p.kind, p.name, p.status,
    p.created_at, p.updated_at, p.decided_at, p.decided_by, p.claimed_by, p.claimed_until,
    p.applied_sha, p.error,
    COALESCE((
        SELECT json_agg(json_build_object('target_path', f.target_path, 'content', f.content) ORDER BY f.target_path)
        FROM proposal_file f
        WHERE f.proposal_id = p.id
    ), '[]') AS files
FROM proposal p`

const getProposalQuery = proposalSelect + `
WHERE p.id = CAST(:id AS uuid)`

const listProposalsQuery = proposalSelect + `
WHERE (CAST(:status AS text) IS NULL OR p.status = CAST(:status AS text))
    AND (CAST(:kind AS text) IS NULL OR p.kind = CAST(:kind AS text))
    AND (CAST(:before AS timestamptz) IS NULL OR p.created_at < CAST(:before AS timestamptz))
ORDER BY p.created_at DESC, p.id
LIMIT :limit`

const lockTargetsQuery = `
SELECT pg_advisory_xact_lock(hashtext(s.t))
FROM (SELECT t FROM unnest(CAST(:targets AS text[])) AS t ORDER BY hashtext(t), t) AS s`

const openTargetsQuery = `
SELECT f.target_path
FROM proposal_file f
JOIN proposal p ON p.id = f.proposal_id
WHERE f.target_path = ANY(CAST(:targets AS text[]))
    AND p.status = ANY(CAST(:open AS text[]))
    AND CAST(p.id AS text) <> :exclude_id
LIMIT 1`

const insertProposalQuery = `
INSERT INTO proposal (source_flow, source_run, kind, name, status)
VALUES (:source_flow, :source_run, :kind, :name, 'pending')
RETURNING CAST(id AS text) AS id`

const insertFilesQuery = `
INSERT INTO proposal_file (proposal_id, target_path, content)
VALUES (:proposal_id, :target_path, :content)`

const insertEventQuery = `
INSERT INTO proposal_event (proposal_id, from_status, to_status, actor, note)
VALUES (CAST(:proposal_id AS uuid), :from_status, :to_status, :actor, :note)`

const listEventsQuery = `
SELECT id, CAST(proposal_id AS text) AS proposal_id, from_status, to_status, actor, note, at
FROM proposal_event
WHERE proposal_id = CAST(:id AS uuid)
ORDER BY at, id`

const transitionQuery = `
UPDATE proposal SET
    status = CAST(:to_status AS text),
    updated_at = now(),
    decided_at = CASE WHEN CAST(:to_status AS text) IN ('approved', 'rejected') THEN now() ELSE decided_at END,
    decided_by = CASE WHEN CAST(:to_status AS text) IN ('approved', 'rejected') THEN CAST(:actor AS text) ELSE decided_by END,
    claimed_by = CASE WHEN CAST(:to_status AS text) = 'applying' THEN CAST(:actor AS text) ELSE NULL END,
    claimed_until = CASE WHEN CAST(:to_status AS text) = 'applying' THEN now() + interval '10 minutes' ELSE NULL END,
    applied_sha = CASE WHEN CAST(:to_status AS text) = 'applied' THEN CAST(:applied_sha AS text) ELSE applied_sha END,
    error = CASE
        WHEN CAST(:to_status AS text) = 'failed' THEN CAST(:note AS text)
        WHEN CAST(:to_status AS text) = 'approved' AND status = 'failed' THEN NULL
        ELSE error
    END
WHERE id = CAST(:id AS uuid) AND status = :from_status
RETURNING CAST(id AS text) AS id`

const proposalExistsQuery = `
SELECT EXISTS (SELECT 1 FROM proposal WHERE id = CAST(:id AS uuid))`

const upsertRejectionQuery = `
INSERT INTO rejection (kind, name, reason)
SELECT kind, name, CAST(:note AS text) FROM proposal WHERE id = CAST(:id AS uuid)
ON CONFLICT (kind, name) DO UPDATE SET reason = EXCLUDED.reason, rejected_at = now()`

const insertRejectionQuery = `
INSERT INTO rejection (kind, name, reason, rejected_at)
VALUES (:kind, :name, :reason, now())
ON CONFLICT (kind, name) DO UPDATE SET reason = EXCLUDED.reason, rejected_at = now()`

const lockProposalQuery = `
SELECT status FROM proposal WHERE id = CAST(:id AS uuid) FOR UPDATE`

const touchProposalQuery = `
UPDATE proposal SET updated_at = now() WHERE id = CAST(:id AS uuid)`

const deleteFilesQuery = `
DELETE FROM proposal_file WHERE proposal_id = CAST(:id AS uuid)`

const resetLeasesQuery = `
WITH reset AS (
    UPDATE proposal
    SET status = 'approved', claimed_by = NULL, claimed_until = NULL, updated_at = now()
    WHERE status = 'applying' AND claimed_until < now()
    RETURNING id
)
INSERT INTO proposal_event (proposal_id, from_status, to_status, actor, note)
SELECT id, 'applying', 'approved', 'lease-expired', NULL FROM reset`

const nextApprovedQuery = `
SELECT CAST(id AS text) AS id
FROM proposal
WHERE status = 'approved'
ORDER BY decided_at, id
LIMIT 1
FOR UPDATE SKIP LOCKED`

const claimProposalQuery = `
UPDATE proposal
SET status = 'applying', claimed_by = :worker,
    claimed_until = now() + make_interval(secs => CAST(:lease_seconds AS double precision)),
    updated_at = now()
WHERE id = CAST(:id AS uuid)`

const expirePendingQuery = `
WITH expired AS (
    UPDATE proposal
    SET status = 'expired', updated_at = now()
    WHERE status = 'pending' AND created_at < now() - make_interval(secs => CAST(:seconds AS double precision))
    RETURNING id
), logged AS (
    INSERT INTO proposal_event (proposal_id, from_status, to_status, actor, note)
    SELECT id, 'pending', 'expired', CAST(:actor AS text), NULL FROM expired
    RETURNING id
)
SELECT count(*) AS count FROM expired`

const rejectionsQuery = `
SELECT kind, name, reason, rejected_at
FROM rejection
WHERE (CAST(:kind AS text) IS NULL OR kind = CAST(:kind AS text))
ORDER BY rejected_at DESC`

type Repository struct {
	db *sqlx.DB
}

var _ pipeline.Repository = (*Repository)(nil)

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func dbError(err error, msg string, attrs ...slog.Attr) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation {
		return domain.Error(domain.ErrNotFound, "proposal not found", attrs...)
	}
	return domain.Error(err, msg, attrs...)
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

func openStatuses() []string {
	statuses := pipeline.OpenStatuses()
	names := make([]string, 0, len(statuses))
	for _, status := range statuses {
		names = append(names, string(status))
	}
	return names
}

func targetsOf(files []pipeline.File) []string {
	targets := make([]string, 0, len(files))
	for _, file := range files {
		targets = append(targets, file.TargetPath)
	}
	return targets
}

func loadProposal(ctx context.Context, ext sqlx.ExtContext, id string) (pipeline.Proposal, error) {
	query, args, err := ext.BindNamed(getProposalQuery, idArgs{ID: id})
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "bind get proposal", slog.String("id", id))
	}
	var rows []proposalRow
	if err := sqlx.SelectContext(ctx, ext, &rows, query, args...); err != nil {
		return pipeline.Proposal{}, dbError(err, "get proposal", slog.String("id", id))
	}
	if len(rows) == 0 {
		return pipeline.Proposal{}, domain.Error(domain.ErrNotFound, "proposal not found", slog.String("id", id))
	}
	return rows[0].toProposal()
}

func guardTargets(ctx context.Context, ext sqlx.ExtContext, files []pipeline.File, excludeID string) error {
	args := targetsArgs{Targets: targetsOf(files), Open: openStatuses(), ExcludeID: excludeID}

	query, bound, err := ext.BindNamed(lockTargetsQuery, args)
	if err != nil {
		return dbError(err, "bind lock targets")
	}
	if _, err := ext.ExecContext(ctx, query, bound...); err != nil {
		return dbError(err, "lock targets")
	}

	query, bound, err = ext.BindNamed(openTargetsQuery, args)
	if err != nil {
		return dbError(err, "bind check open targets")
	}
	var conflicts []string
	if err := sqlx.SelectContext(ctx, ext, &conflicts, query, bound...); err != nil {
		return dbError(err, "check open targets")
	}
	if len(conflicts) > 0 {
		return domain.Error(domain.ErrConflict, "open proposal already targets this path", slog.String("target_path", conflicts[0]))
	}
	return nil
}

func insertFiles(ctx context.Context, ext sqlx.ExtContext, id string, files []pipeline.File) error {
	query, args, err := ext.BindNamed(insertFilesQuery, filesToRows(id, files))
	if err != nil {
		return dbError(err, "bind insert files", slog.String("id", id))
	}
	if _, err := ext.ExecContext(ctx, query, args...); err != nil {
		return dbError(err, "insert files", slog.String("id", id))
	}
	return nil
}

func insertEvent(ctx context.Context, ext sqlx.ExtContext, event eventRow) error {
	query, args, err := ext.BindNamed(insertEventQuery, event)
	if err != nil {
		return dbError(err, "bind insert event", slog.String("id", event.ProposalID))
	}
	if _, err := ext.ExecContext(ctx, query, args...); err != nil {
		return dbError(err, "insert event", slog.String("id", event.ProposalID))
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, p pipeline.NewProposal) (pipeline.Proposal, error) {
	if len(p.Files) == 0 {
		return pipeline.Proposal{}, domain.Error(domain.ErrValidation, "proposal has no files")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "begin transaction")
	}
	created, err := createTx(ctx, tx, p)
	if err := finish(tx, err); err != nil {
		return pipeline.Proposal{}, err
	}
	return created, nil
}

func createTx(ctx context.Context, tx *sqlx.Tx, p pipeline.NewProposal) (pipeline.Proposal, error) {
	if err := guardTargets(ctx, tx, p.Files, ""); err != nil {
		return pipeline.Proposal{}, err
	}

	query, args, err := tx.BindNamed(insertProposalQuery, createArgs{
		SourceFlow: p.SourceFlow,
		SourceRun:  p.SourceRun,
		Kind:       string(p.Kind),
		Name:       p.Name,
	})
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "bind insert proposal")
	}
	var ids []idRow
	if err := sqlx.SelectContext(ctx, tx, &ids, query, args...); err != nil {
		return pipeline.Proposal{}, dbError(err, "insert proposal", slog.String("name", p.Name))
	}
	if len(ids) != 1 {
		return pipeline.Proposal{}, domain.Error(domain.ErrInternal, "insert proposal returned no row", slog.String("name", p.Name))
	}
	id := ids[0].ID

	if err := insertFiles(ctx, tx, id, p.Files); err != nil {
		return pipeline.Proposal{}, err
	}
	to := string(pipeline.StatusPending)
	event := eventRow{ProposalID: id, ToStatus: to, Actor: p.SourceFlow}
	if err := insertEvent(ctx, tx, event); err != nil {
		return pipeline.Proposal{}, err
	}
	return loadProposal(ctx, tx, id)
}

func (r *Repository) Get(ctx context.Context, id string) (pipeline.Proposal, error) {
	return loadProposal(ctx, r.db, id)
}

func (r *Repository) List(ctx context.Context, f pipeline.ListFilter) ([]pipeline.Proposal, error) {
	args := listArgs{Before: f.Before, Limit: f.Limit}
	if f.Status != nil {
		status := string(*f.Status)
		args.Status = &status
	}
	if f.Kind != nil {
		kind := string(*f.Kind)
		args.Kind = &kind
	}
	query, bound, err := r.db.BindNamed(listProposalsQuery, args)
	if err != nil {
		return nil, dbError(err, "bind list proposals")
	}
	var rows []proposalRow
	if err := r.db.SelectContext(ctx, &rows, query, bound...); err != nil {
		return nil, dbError(err, "list proposals")
	}
	proposals := make([]pipeline.Proposal, 0, len(rows))
	for _, row := range rows {
		proposal, err := row.toProposal()
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func (r *Repository) Events(ctx context.Context, id string) ([]pipeline.Event, error) {
	query, args, err := r.db.BindNamed(listEventsQuery, idArgs{ID: id})
	if err != nil {
		return nil, dbError(err, "bind list events", slog.String("id", id))
	}
	var rows []eventRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, dbError(err, "list events", slog.String("id", id))
	}
	events := make([]pipeline.Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, row.toEvent())
	}
	return events, nil
}

func (r *Repository) Transition(ctx context.Context, t pipeline.Transition) (pipeline.Proposal, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "begin transaction")
	}
	moved, err := transitionTx(ctx, tx, t)
	if err := finish(tx, err); err != nil {
		return pipeline.Proposal{}, err
	}
	return moved, nil
}

func transitionTx(ctx context.Context, tx *sqlx.Tx, t pipeline.Transition) (pipeline.Proposal, error) {
	args := transitionArgs{
		ID:         t.ID,
		FromStatus: string(t.From),
		ToStatus:   string(t.To),
		Actor:      t.Actor,
		Note:       t.Note,
		AppliedSHA: t.AppliedSHA,
	}
	query, bound, err := tx.BindNamed(transitionQuery, args)
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "bind transition", slog.String("id", t.ID))
	}
	var ids []idRow
	if err := sqlx.SelectContext(ctx, tx, &ids, query, bound...); err != nil {
		return pipeline.Proposal{}, dbError(err, "transition proposal", slog.String("id", t.ID))
	}
	if len(ids) == 0 {
		return pipeline.Proposal{}, transitionMiss(ctx, tx, t)
	}

	from := string(t.From)
	event := eventRow{ProposalID: t.ID, FromStatus: &from, ToStatus: string(t.To), Actor: t.Actor, Note: t.Note}
	if err := insertEvent(ctx, tx, event); err != nil {
		return pipeline.Proposal{}, err
	}
	if t.To == pipeline.StatusRejected {
		query, bound, err := tx.BindNamed(upsertRejectionQuery, args)
		if err != nil {
			return pipeline.Proposal{}, dbError(err, "bind upsert rejection", slog.String("id", t.ID))
		}
		if _, err := tx.ExecContext(ctx, query, bound...); err != nil {
			return pipeline.Proposal{}, dbError(err, "upsert rejection", slog.String("id", t.ID))
		}
	}
	return loadProposal(ctx, tx, t.ID)
}

func transitionMiss(ctx context.Context, tx *sqlx.Tx, t pipeline.Transition) error {
	query, args, err := tx.BindNamed(proposalExistsQuery, idArgs{ID: t.ID})
	if err != nil {
		return dbError(err, "bind proposal exists", slog.String("id", t.ID))
	}
	var exists bool
	if err := sqlx.GetContext(ctx, tx, &exists, query, args...); err != nil {
		return dbError(err, "check proposal exists", slog.String("id", t.ID))
	}
	if !exists {
		return domain.Error(domain.ErrNotFound, "proposal not found", slog.String("id", t.ID))
	}
	return domain.Error(domain.ErrConflict, "proposal is not in the expected status",
		slog.String("id", t.ID), slog.String("from", string(t.From)), slog.String("to", string(t.To)))
}

func (r *Repository) ReplaceFiles(ctx context.Context, id, actor string, files []pipeline.File) (pipeline.Proposal, error) {
	if len(files) == 0 {
		return pipeline.Proposal{}, domain.Error(domain.ErrValidation, "proposal has no files", slog.String("id", id))
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "begin transaction")
	}
	replaced, err := replaceFilesTx(ctx, tx, id, actor, files)
	if err := finish(tx, err); err != nil {
		return pipeline.Proposal{}, err
	}
	return replaced, nil
}

func lockPending(ctx context.Context, tx *sqlx.Tx, id string) error {
	query, args, err := tx.BindNamed(lockProposalQuery, idArgs{ID: id})
	if err != nil {
		return dbError(err, "bind lock proposal", slog.String("id", id))
	}
	var statuses []statusRow
	if err := sqlx.SelectContext(ctx, tx, &statuses, query, args...); err != nil {
		return dbError(err, "lock proposal", slog.String("id", id))
	}
	if len(statuses) == 0 {
		return domain.Error(domain.ErrNotFound, "proposal not found", slog.String("id", id))
	}
	if pipeline.Status(statuses[0].Status) != pipeline.StatusPending {
		return domain.Error(domain.ErrConflict, "proposal is not pending",
			slog.String("id", id), slog.String("status", statuses[0].Status))
	}
	return nil
}

func replaceFilesTx(ctx context.Context, tx *sqlx.Tx, id, actor string, files []pipeline.File) (pipeline.Proposal, error) {
	if err := lockPending(ctx, tx, id); err != nil {
		return pipeline.Proposal{}, err
	}

	if err := guardTargets(ctx, tx, files, id); err != nil {
		return pipeline.Proposal{}, err
	}

	for _, statement := range []string{deleteFilesQuery, touchProposalQuery} {
		query, args, err := tx.BindNamed(statement, idArgs{ID: id})
		if err != nil {
			return pipeline.Proposal{}, dbError(err, "bind replace files", slog.String("id", id))
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return pipeline.Proposal{}, dbError(err, "replace files", slog.String("id", id))
		}
	}
	if err := insertFiles(ctx, tx, id, files); err != nil {
		return pipeline.Proposal{}, err
	}

	pending := string(pipeline.StatusPending)
	note := "revised"
	event := eventRow{ProposalID: id, FromStatus: &pending, ToStatus: pending, Actor: actor, Note: &note}
	if err := insertEvent(ctx, tx, event); err != nil {
		return pipeline.Proposal{}, err
	}
	return loadProposal(ctx, tx, id)
}

func (r *Repository) Claim(ctx context.Context, worker string, lease time.Duration) (pipeline.Proposal, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "begin transaction")
	}
	claimed, err := claimTx(ctx, tx, worker, lease)
	if err := finish(tx, err); err != nil {
		return pipeline.Proposal{}, err
	}
	return claimed, nil
}

func claimTx(ctx context.Context, tx *sqlx.Tx, worker string, lease time.Duration) (pipeline.Proposal, error) {
	if _, err := tx.ExecContext(ctx, resetLeasesQuery); err != nil {
		return pipeline.Proposal{}, dbError(err, "reset expired leases")
	}

	var next []idRow
	if err := tx.SelectContext(ctx, &next, nextApprovedQuery); err != nil {
		return pipeline.Proposal{}, dbError(err, "select next approved proposal")
	}
	if len(next) == 0 {
		return pipeline.Proposal{}, pipeline.ErrNothingToClaim
	}
	id := next[0].ID

	query, args, err := tx.BindNamed(claimProposalQuery, claimArgs{ID: id, Worker: worker, LeaseSeconds: lease.Seconds()})
	if err != nil {
		return pipeline.Proposal{}, dbError(err, "bind claim proposal", slog.String("id", id))
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return pipeline.Proposal{}, dbError(err, "claim proposal", slog.String("id", id))
	}

	approved := string(pipeline.StatusApproved)
	event := eventRow{ProposalID: id, FromStatus: &approved, ToStatus: string(pipeline.StatusApplying), Actor: worker}
	if err := insertEvent(ctx, tx, event); err != nil {
		return pipeline.Proposal{}, err
	}
	claimed, err := loadProposal(ctx, tx, id)
	if err != nil {
		return pipeline.Proposal{}, err
	}
	return claimed, nil
}

func (r *Repository) ExpirePending(ctx context.Context, olderThan time.Duration, actor string) (int, error) {
	query, args, err := r.db.BindNamed(expirePendingQuery, expireArgs{Seconds: olderThan.Seconds(), Actor: actor})
	if err != nil {
		return 0, dbError(err, "bind expire pending")
	}
	var count countRow
	if err := r.db.GetContext(ctx, &count, query, args...); err != nil {
		return 0, dbError(err, "expire pending proposals")
	}
	return count.Count, nil
}

func (r *Repository) Rejections(ctx context.Context, kind *pipeline.Kind) ([]pipeline.Rejection, error) {
	args := rejectionsArgs{}
	if kind != nil {
		name := string(*kind)
		args.Kind = &name
	}
	query, bound, err := r.db.BindNamed(rejectionsQuery, args)
	if err != nil {
		return nil, dbError(err, "bind list rejections")
	}
	var rows []rejectionRow
	if err := r.db.SelectContext(ctx, &rows, query, bound...); err != nil {
		return nil, dbError(err, "list rejections")
	}
	rejections := make([]pipeline.Rejection, 0, len(rows))
	for _, row := range rows {
		rejections = append(rejections, row.toRejection())
	}
	return rejections, nil
}

func (r *Repository) UpsertRejection(ctx context.Context, rej pipeline.Rejection) error {
	args := upsertRejectionArgs{Kind: string(rej.Kind), Name: rej.Name, Reason: rej.Reason}
	query, bound, err := r.db.BindNamed(insertRejectionQuery, args)
	if err != nil {
		return dbError(err, "bind upsert rejection", slog.String("name", rej.Name))
	}
	if _, err := r.db.ExecContext(ctx, query, bound...); err != nil {
		return dbError(err, "upsert rejection", slog.String("name", rej.Name))
	}
	return nil
}
