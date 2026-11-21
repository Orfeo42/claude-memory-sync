package runs

import "claude-memory-sync/internal/domain"

type Trigger string

const (
	TriggerSchedule Trigger = "schedule"
	TriggerManual   Trigger = "manual"
	TriggerEvent    Trigger = "event"
)

func ParseTrigger(s string) (Trigger, error) {
	switch trigger := Trigger(s); trigger {
	case TriggerSchedule, TriggerManual, TriggerEvent:
		return trigger, nil
	default:
		return "", domain.Error(domain.ErrValidation, "unknown run trigger")
	}
}

type Status string

const (
	StatusRunning     Status = "running"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusPartial     Status = "partial"
	StatusInterrupted Status = "interrupted"
)

func ParseStatus(s string) (Status, error) {
	switch status := Status(s); status {
	case StatusRunning, StatusSucceeded, StatusFailed, StatusPartial, StatusInterrupted:
		return status, nil
	default:
		return "", domain.Error(domain.ErrValidation, "unknown run status")
	}
}

type StepStatus string

const (
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
	StepLossy     StepStatus = "lossy"
)

func ParseStepStatus(s string) (StepStatus, error) {
	switch status := StepStatus(s); status {
	case StepSucceeded, StepFailed, StepSkipped, StepLossy:
		return status, nil
	default:
		return "", domain.Error(domain.ErrValidation, "unknown run step status")
	}
}
