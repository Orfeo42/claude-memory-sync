package synthsettings

import (
	"context"
	"log/slog"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows/prompts"
	"claude-memory-sync/internal/settings"
)

const maxPromptChars = 200000

func buildPrompt(name, def string, override settings.Setting, overridden bool) Prompt {
	if !overridden {
		return Prompt{Name: name, Content: def, Default: def}
	}
	updated := override.UpdatedAt
	return Prompt{Name: name, Content: *override.Value, Default: def, Overridden: true, UpdatedAt: &updated}
}

func (s *Service) Prompt(ctx context.Context, name string) (string, error) {
	p, err := s.GetPrompt(ctx, name)
	if err != nil {
		return "", err
	}
	return p.Content, nil
}

func (s *Service) Prompts(ctx context.Context) ([]PromptInfo, error) {
	byKey, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	names := prompts.Names()
	infos := make([]PromptInfo, 0, len(names))
	for _, name := range names {
		info := PromptInfo{Name: name}
		if override, ok := byKey[promptPrefix+name]; ok {
			updated := override.UpdatedAt
			info.Overridden = true
			info.UpdatedAt = &updated
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (s *Service) GetPrompt(ctx context.Context, name string) (Prompt, error) {
	def, err := prompts.Default(name)
	if err != nil {
		return Prompt{}, err
	}
	override, overridden, err := s.lookup(ctx, promptPrefix+name)
	if err != nil {
		return Prompt{}, err
	}
	return buildPrompt(name, def, override, overridden), nil
}

func (s *Service) SetPrompt(ctx context.Context, name, content string) (Prompt, error) {
	def, err := prompts.Default(name)
	if err != nil {
		return Prompt{}, err
	}
	if content == "" || len(content) > maxPromptChars {
		return Prompt{}, domain.Error(domain.ErrValidation, "prompt content must be 1-200000 characters", slog.String("key", promptPrefix+name))
	}
	override, err := s.put(ctx, promptPrefix+name, content)
	if err != nil {
		return Prompt{}, err
	}
	override.Value = &content
	return buildPrompt(name, def, override, true), nil
}

func (s *Service) ResetPrompt(ctx context.Context, name string) (Prompt, error) {
	def, err := prompts.Default(name)
	if err != nil {
		return Prompt{}, err
	}
	if err := s.remove(ctx, promptPrefix+name); err != nil {
		return Prompt{}, err
	}
	return buildPrompt(name, def, settings.Setting{}, false), nil
}

func (s *Service) PromptHistory(ctx context.Context, name string, limit int) ([]PromptVersion, error) {
	if _, err := prompts.Default(name); err != nil {
		return nil, err
	}
	key := promptPrefix + name
	history, err := s.store.SettingHistory(ctx, key, limit)
	if err != nil {
		return nil, domain.WrapError(err, slog.String("key", key))
	}
	versions := make([]PromptVersion, 0, len(history))
	for _, v := range history {
		versions = append(versions, PromptVersion{Content: v.Value, Deleted: v.Deleted, CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy})
	}
	return versions, nil
}
