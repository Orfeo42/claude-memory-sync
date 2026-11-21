package llm

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"claude-memory-sync/internal/domain"
)

const (
	defaultBinary  = "claude"
	defaultTimeout = 10 * time.Minute
	stderrTailSize = 400
	anthropicEnv   = "ANTHROPIC_API_KEY"
	oauthEnv       = "CLAUDE_CODE_OAUTH_TOKEN"
)

//go:generate go tool moq -out llm_mock.go . TokenSource:MockTokenSource
type TokenSource interface {
	ClaudeOAuthToken(ctx context.Context) (string, error)
}

type Request struct {
	Model  string
	Prompt string
}

type Runner struct {
	binary  string
	tokens  TokenSource
	timeout time.Duration
}

type Option func(*Runner)

func WithBinary(path string) Option {
	return func(r *Runner) { r.binary = path }
}

func WithTimeout(d time.Duration) Option {
	return func(r *Runner) { r.timeout = d }
}

func New(tokens TokenSource, opts ...Option) *Runner {
	r := &Runner{binary: defaultBinary, tokens: tokens, timeout: defaultTimeout}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Runner) Run(ctx context.Context, req Request) (string, error) {
	if os.Getenv(anthropicEnv) != "" {
		return "", domain.WrapError(ErrAnthropicEnvSet)
	}
	switch req.Model {
	case "sonnet", "opus", "haiku":
	default:
		return "", domain.Error(domain.ErrValidation, "unsupported model", slog.String("model", req.Model))
	}
	if req.Prompt == "" {
		return "", domain.Error(domain.ErrValidation, "empty prompt")
	}

	token, err := r.tokens.ClaudeOAuthToken(ctx)
	if err != nil {
		return "", domain.Error(err, "load claude oauth token")
	}
	if token == "" {
		return "", domain.Error(domain.ErrValidation, "claude oauth token not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binary, "-p", "--model", req.Model)
	cmd.Stdin = strings.NewReader(req.Prompt)
	cmd.Env = childEnv(token)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", domain.Error(&domain.UpstreamError{
			API:        "claude",
			StatusCode: http.StatusBadGateway,
			Body:       stderrTail(stderr.String(), token),
		}, "run claude cli", slog.String("cause", err.Error()))
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", domain.Error(&domain.UpstreamError{
			API:        "claude",
			StatusCode: http.StatusBadGateway,
			Body:       stderrTail(stderr.String(), token),
		}, "claude cli returned empty output")
	}
	return out, nil
}

func childEnv(token string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, anthropicEnv+"=") || strings.HasPrefix(kv, oauthEnv+"=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, oauthEnv+"="+token)
}

func stderrTail(stderr, token string) string {
	redacted := strings.ReplaceAll(stderr, token, "[redacted]")
	if len(redacted) > stderrTailSize {
		return redacted[len(redacted)-stderrTailSize:]
	}
	return redacted
}
