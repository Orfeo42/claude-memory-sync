package synthsettings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/settings"
)

const Actor = "ui"

type ParamKind string

const (
	KindModel ParamKind = "model"
	KindBool  ParamKind = "bool"
	KindInt   ParamKind = "int"
)

type Param struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Default     string    `json:"default"`
	Kind        ParamKind `json:"kind"`
	Description string    `json:"description"`
	Overridden  bool      `json:"overridden"`
}

type PromptInfo struct {
	Name       string     `json:"name"`
	Overridden bool       `json:"overridden"`
	UpdatedAt  *time.Time `json:"updated_at"`
}

type Prompt struct {
	Name       string     `json:"name"`
	Content    string     `json:"content"`
	Default    string     `json:"default"`
	Overridden bool       `json:"overridden"`
	UpdatedAt  *time.Time `json:"updated_at"`
}

type PromptVersion struct {
	Content   *string   `json:"content"`
	Deleted   bool      `json:"deleted"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

type TokenStatus struct {
	Set       bool       `json:"set"`
	UpdatedAt *time.Time `json:"updated_at"`
}

//go:generate go tool moq -out store_mock.go . Store:MockStore

type Store interface {
	ListSettings(ctx context.Context) ([]settings.Setting, error)
	GetSetting(ctx context.Context, key string) (settings.Setting, error)
	PutSetting(ctx context.Context, key string, req client.PutSettingRequest) (settings.Setting, error)
	DeleteSetting(ctx context.Context, key, actor string) error
	SettingHistory(ctx context.Context, key string, limit int) ([]settings.Version, error)
}

type Service struct {
	store    Store
	envToken string
}

func New(store Store, envToken string) *Service {
	return &Service{store: store, envToken: envToken}
}

const (
	promptPrefix   = "prompt."
	paramPrefix    = "param."
	schedulePrefix = "schedule."
	tokenKey       = "secret.claude-oauth-token"
)

func DefaultSchedule(flow flows.Name) string {
	switch flow {
	case flows.Intake:
		return "*/45 * * * *"
	case flows.DailySynth:
		return "0 3 * * *"
	case flows.Taskmine:
		return "0 4 * * 0"
	case flows.ProposalApply:
		return "*/10 * * * *"
	}
	return ""
}

func (s *Service) lookup(ctx context.Context, key string) (settings.Setting, bool, error) {
	setting, err := s.store.GetSetting(ctx, key)
	if errors.Is(err, domain.ErrNotFound) {
		return settings.Setting{}, false, nil
	}
	if err != nil {
		return settings.Setting{}, false, domain.WrapError(err, slog.String("key", key))
	}
	return setting, setting.Set && setting.Value != nil, nil
}

func (s *Service) list(ctx context.Context) (map[string]settings.Setting, error) {
	all, err := s.store.ListSettings(ctx)
	if err != nil {
		return nil, domain.WrapError(err, slog.String("operation", "list_settings"))
	}
	byKey := make(map[string]settings.Setting, len(all))
	for _, setting := range all {
		if setting.Set && setting.Value != nil {
			byKey[setting.Key] = setting
		}
	}
	return byKey, nil
}

func (s *Service) put(ctx context.Context, key, value string) (settings.Setting, error) {
	setting, err := s.store.PutSetting(ctx, key, client.PutSettingRequest{Value: value, Actor: Actor})
	if err != nil {
		return settings.Setting{}, domain.WrapError(err, slog.String("key", key))
	}
	return setting, nil
}

func (s *Service) remove(ctx context.Context, key string) error {
	err := s.store.DeleteSetting(ctx, key, Actor)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.WrapError(err, slog.String("key", key))
	}
	return nil
}
