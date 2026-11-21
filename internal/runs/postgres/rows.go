package postgres

import (
	"encoding/json"
	"time"

	"claude-memory-sync/internal/runs"
)

type runRow struct {
	ID         string     `db:"id"`
	Flow       string     `db:"flow"`
	Trigger    string     `db:"trigger"`
	Status     string     `db:"status"`
	StartedAt  time.Time  `db:"started_at"`
	FinishedAt *time.Time `db:"finished_at"`
	Error      *string    `db:"error"`
	Summary    []byte     `db:"summary"`
}

type stepRow struct {
	ID         int64     `db:"id"`
	RunID      string    `db:"run_id"`
	Step       string    `db:"step"`
	Status     string    `db:"status"`
	StartedAt  time.Time `db:"started_at"`
	FinishedAt time.Time `db:"finished_at"`
	Error      *string   `db:"error"`
	OutputTail *string   `db:"output_tail"`
}

type idArgs struct {
	ID string `db:"id"`
}

type createArgs struct {
	Flow    string `db:"flow"`
	Trigger string `db:"trigger"`
}

type stepArgs struct {
	RunID      string    `db:"run_id"`
	Step       string    `db:"step"`
	Status     string    `db:"status"`
	StartedAt  time.Time `db:"started_at"`
	FinishedAt time.Time `db:"finished_at"`
	Error      *string   `db:"error"`
	OutputTail *string   `db:"output_tail"`
}

type finishArgs struct {
	ID      string  `db:"id"`
	Status  string  `db:"status"`
	Error   *string `db:"error"`
	Summary *string `db:"summary"`
}

type interruptArgs struct {
	Flow    *string `db:"flow"`
	Message string  `db:"message"`
}

type listArgs struct {
	Flow   *string    `db:"flow"`
	Status *string    `db:"status"`
	Before *time.Time `db:"before"`
	Limit  int        `db:"limit"`
}

type pruneArgs struct {
	Seconds float64 `db:"seconds"`
}

type statusRow struct {
	Status string `db:"status"`
}

func (r runRow) toRun() runs.Run {
	return runs.Run{
		ID:         r.ID,
		Flow:       r.Flow,
		Trigger:    runs.Trigger(r.Trigger),
		Status:     runs.Status(r.Status),
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Error:      r.Error,
		Summary:    json.RawMessage(r.Summary),
	}
}

func (r stepRow) toStep() runs.Step {
	return runs.Step{
		ID:         r.ID,
		RunID:      r.RunID,
		Step:       r.Step,
		Status:     runs.StepStatus(r.Status),
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Error:      r.Error,
		OutputTail: r.OutputTail,
	}
}
