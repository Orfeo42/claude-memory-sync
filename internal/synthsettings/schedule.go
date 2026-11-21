package synthsettings

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
)

func (s *Service) Schedule(ctx context.Context, flow flows.Name) (string, error) {
	if _, err := flows.ParseName(string(flow)); err != nil {
		return "", err
	}
	override, overridden, err := s.lookup(ctx, schedulePrefix+string(flow))
	if err != nil {
		return "", err
	}
	if !overridden {
		return DefaultSchedule(flow), nil
	}
	return *override.Value, nil
}

func (s *Service) SetSchedule(ctx context.Context, flow flows.Name, spec string) (string, error) {
	if _, err := flows.ParseName(string(flow)); err != nil {
		return "", err
	}
	key := schedulePrefix + string(flow)
	if _, err := cron.ParseStandard(spec); err != nil {
		return "", domain.Error(domain.ErrValidation, "invalid cron expression", slog.String("key", key))
	}
	if _, err := s.put(ctx, key, spec); err != nil {
		return "", err
	}
	return spec, nil
}

func (s *Service) ResetSchedule(ctx context.Context, flow flows.Name) (string, error) {
	if _, err := flows.ParseName(string(flow)); err != nil {
		return "", err
	}
	if err := s.remove(ctx, schedulePrefix+string(flow)); err != nil {
		return "", err
	}
	return DefaultSchedule(flow), nil
}
