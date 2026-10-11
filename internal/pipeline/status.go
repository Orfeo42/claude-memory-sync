package pipeline

import (
	"slices"

	"claude-memory-sync/internal/domain"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusApplying Status = "applying"
	StatusApplied  Status = "applied"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
	StatusFailed   Status = "failed"
)

func ParseStatus(s string) (Status, error) {
	switch status := Status(s); status {
	case StatusPending, StatusApproved, StatusApplying, StatusApplied, StatusRejected, StatusExpired, StatusFailed:
		return status, nil
	default:
		return "", domain.Error(domain.ErrValidation, "unknown proposal status")
	}
}

func CanTransition(from, to Status) bool {
	return slices.Contains(nextStatuses(from), to)
}

func nextStatuses(from Status) []Status {
	switch from {
	case StatusPending:
		return []Status{StatusApproved, StatusRejected, StatusExpired}
	case StatusApproved:
		return []Status{StatusApplying}
	case StatusApplying:
		return []Status{StatusApplied, StatusFailed, StatusApproved}
	case StatusFailed:
		return []Status{StatusApproved}
	default:
		return nil
	}
}

func OpenStatuses() []Status {
	return []Status{StatusPending, StatusApproved, StatusApplying}
}
