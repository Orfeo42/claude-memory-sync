package postgres

import (
	"encoding/json"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
)

type proposalRow struct {
	ID           string     `db:"id"`
	SourceFlow   string     `db:"source_flow"`
	SourceRun    string     `db:"source_run"`
	Kind         string     `db:"kind"`
	Name         string     `db:"name"`
	Status       string     `db:"status"`
	CreatedAt    time.Time  `db:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at"`
	DecidedAt    *time.Time `db:"decided_at"`
	DecidedBy    *string    `db:"decided_by"`
	ClaimedBy    *string    `db:"claimed_by"`
	ClaimedUntil *time.Time `db:"claimed_until"`
	AppliedSHA   *string    `db:"applied_sha"`
	Error        *string    `db:"error"`
	Files        []byte     `db:"files"`
}

type fileRow struct {
	ProposalID string `db:"proposal_id" json:"-"`
	TargetPath string `db:"target_path" json:"target_path"`
	Content    string `db:"content" json:"content"`
}

type eventRow struct {
	ID         int64     `db:"id"`
	ProposalID string    `db:"proposal_id"`
	FromStatus *string   `db:"from_status"`
	ToStatus   string    `db:"to_status"`
	Actor      string    `db:"actor"`
	Note       *string   `db:"note"`
	At         time.Time `db:"at"`
}

type rejectionRow struct {
	Kind       string    `db:"kind"`
	Name       string    `db:"name"`
	Reason     *string   `db:"reason"`
	RejectedAt time.Time `db:"rejected_at"`
}

type idRow struct {
	ID string `db:"id"`
}

type statusRow struct {
	Status string `db:"status"`
}

type idArgs struct {
	ID string `db:"id"`
}

type createArgs struct {
	SourceFlow string `db:"source_flow"`
	SourceRun  string `db:"source_run"`
	Kind       string `db:"kind"`
	Name       string `db:"name"`
}

type listArgs struct {
	Status *string    `db:"status"`
	Kind   *string    `db:"kind"`
	Before *time.Time `db:"before"`
	Limit  int        `db:"limit"`
}

type targetsArgs struct {
	Targets   []string `db:"targets"`
	Open      []string `db:"open"`
	ExcludeID string   `db:"exclude_id"`
}

type transitionArgs struct {
	ID         string  `db:"id"`
	FromStatus string  `db:"from_status"`
	ToStatus   string  `db:"to_status"`
	Actor      string  `db:"actor"`
	Note       *string `db:"note"`
	AppliedSHA *string `db:"applied_sha"`
}

type claimArgs struct {
	ID           string  `db:"id"`
	Worker       string  `db:"worker"`
	LeaseSeconds float64 `db:"lease_seconds"`
}

type expireArgs struct {
	Seconds float64 `db:"seconds"`
	Actor   string  `db:"actor"`
}

type rejectionsArgs struct {
	Kind *string `db:"kind"`
}

type upsertRejectionArgs struct {
	Kind   string  `db:"kind"`
	Name   string  `db:"name"`
	Reason *string `db:"reason"`
}

type countRow struct {
	Count int `db:"count"`
}

func (r proposalRow) toProposal() (pipeline.Proposal, error) {
	var files []fileRow
	if err := json.Unmarshal(r.Files, &files); err != nil {
		return pipeline.Proposal{}, domain.Error(err, "decode proposal files")
	}
	return pipeline.Proposal{
		ID:           r.ID,
		SourceFlow:   r.SourceFlow,
		SourceRun:    r.SourceRun,
		Kind:         pipeline.Kind(r.Kind),
		Name:         r.Name,
		Status:       pipeline.Status(r.Status),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		DecidedAt:    r.DecidedAt,
		DecidedBy:    r.DecidedBy,
		ClaimedBy:    r.ClaimedBy,
		ClaimedUntil: r.ClaimedUntil,
		AppliedSHA:   r.AppliedSHA,
		Error:        r.Error,
		Files:        filesFromRows(files),
	}, nil
}

func filesFromRows(rows []fileRow) []pipeline.File {
	files := make([]pipeline.File, 0, len(rows))
	for _, row := range rows {
		files = append(files, pipeline.File{TargetPath: row.TargetPath, Content: row.Content})
	}
	return files
}

func filesToRows(proposalID string, files []pipeline.File) []fileRow {
	rows := make([]fileRow, 0, len(files))
	for _, file := range files {
		rows = append(rows, fileRow{ProposalID: proposalID, TargetPath: file.TargetPath, Content: file.Content})
	}
	return rows
}

func (r eventRow) toEvent() pipeline.Event {
	event := pipeline.Event{
		ID:         r.ID,
		ProposalID: r.ProposalID,
		ToStatus:   pipeline.Status(r.ToStatus),
		Actor:      r.Actor,
		Note:       r.Note,
		At:         r.At,
	}
	if r.FromStatus != nil {
		from := pipeline.Status(*r.FromStatus)
		event.FromStatus = &from
	}
	return event
}

func (r rejectionRow) toRejection() pipeline.Rejection {
	return pipeline.Rejection{
		Kind:       pipeline.Kind(r.Kind),
		Name:       r.Name,
		Reason:     r.Reason,
		RejectedAt: r.RejectedAt,
	}
}
