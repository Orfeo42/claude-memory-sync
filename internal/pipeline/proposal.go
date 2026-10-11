package pipeline

import "time"

type File struct {
	TargetPath string `json:"target_path"`
	Content    string `json:"content"`
}

type Proposal struct {
	ID           string     `json:"id"`
	SourceFlow   string     `json:"source_flow"`
	SourceRun    string     `json:"source_run"`
	Kind         Kind       `json:"kind"`
	Name         string     `json:"name"`
	Status       Status     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	DecidedBy    *string    `json:"decided_by,omitempty"`
	ClaimedBy    *string    `json:"claimed_by,omitempty"`
	ClaimedUntil *time.Time `json:"claimed_until,omitempty"`
	AppliedSHA   *string    `json:"applied_sha,omitempty"`
	Error        *string    `json:"error,omitempty"`
	Files        []File     `json:"files"`
}

type ProposalDetail struct {
	Proposal Proposal `json:"proposal"`
	Events   []Event  `json:"events"`
}

type Event struct {
	ID         int64     `json:"id"`
	ProposalID string    `json:"proposal_id"`
	FromStatus *Status   `json:"from_status,omitempty"`
	ToStatus   Status    `json:"to_status"`
	Actor      string    `json:"actor"`
	Note       *string   `json:"note,omitempty"`
	At         time.Time `json:"at"`
}

type Rejection struct {
	Kind       Kind      `json:"kind"`
	Name       string    `json:"name"`
	Reason     *string   `json:"reason,omitempty"`
	RejectedAt time.Time `json:"rejected_at"`
}

type NewProposal struct {
	SourceFlow string
	SourceRun  string
	Kind       Kind
	Name       string
	Files      []File
}

type ListFilter struct {
	Status *Status
	Kind   *Kind
	Before *time.Time
	Limit  int
}

type Transition struct {
	ID         string
	From       Status
	To         Status
	Actor      string
	Note       *string
	AppliedSHA *string
}
