package synthsettings

import (
	"context"
	"log/slog"
	"strings"

	"claude-memory-sync/internal/domain"
)

const maxTokenChars = 4096

func (s *Service) SetToken(ctx context.Context, token string) (TokenStatus, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenChars {
		return TokenStatus{}, domain.Error(domain.ErrValidation, "token must be 1-4096 characters", slog.String("key", tokenKey))
	}
	stored, err := s.put(ctx, tokenKey, token)
	if err != nil {
		return TokenStatus{}, err
	}
	updated := stored.UpdatedAt
	return TokenStatus{Set: true, UpdatedAt: &updated}, nil
}

func (s *Service) ClearToken(ctx context.Context) (TokenStatus, error) {
	if err := s.remove(ctx, tokenKey); err != nil {
		return TokenStatus{}, err
	}
	return s.TokenStatus(ctx)
}

func (s *Service) TokenStatus(ctx context.Context) (TokenStatus, error) {
	stored, found, err := s.lookup(ctx, tokenKey)
	if err != nil {
		return TokenStatus{}, err
	}
	if found {
		updated := stored.UpdatedAt
		return TokenStatus{Set: true, UpdatedAt: &updated}, nil
	}
	return TokenStatus{Set: s.envToken != ""}, nil
}

func (s *Service) ClaudeOAuthToken(ctx context.Context) (string, error) {
	stored, found, err := s.lookup(ctx, tokenKey)
	if err != nil {
		return "", err
	}
	if found {
		return *stored.Value, nil
	}
	return s.envToken, nil
}
