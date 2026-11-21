package runs

import (
	"encoding/json"
	"time"
)

type Run struct {
	ID         string          `json:"id"`
	Flow       string          `json:"flow"`
	Trigger    Trigger         `json:"trigger"`
	Status     Status          `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Error      *string         `json:"error,omitempty"`
	Summary    json.RawMessage `json:"summary" type:"object"`
}

type Step struct {
	ID         int64      `json:"id"`
	RunID      string     `json:"run_id"`
	Step       string     `json:"step"`
	Status     StepStatus `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt time.Time  `json:"finished_at"`
	Error      *string    `json:"error,omitempty"`
	OutputTail *string    `json:"output_tail,omitempty"`
}

type RunDetail struct {
	Run   Run    `json:"run"`
	Steps []Step `json:"steps"`
}

type NewRun struct {
	Flow    string
	Trigger Trigger
}

type NewStep struct {
	RunID      string
	Step       string
	Status     StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Error      *string
	OutputTail *string
}

type Finish struct {
	ID      string
	Status  Status
	Error   *string
	Summary json.RawMessage
}

type ListFilter struct {
	Flow   *string
	Status *Status
	Before *time.Time
	Limit  int
}
