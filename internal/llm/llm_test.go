package llm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/llm"
)

const fakeScript = `#!/usr/bin/env bash
printf '%s\n' "$@" > "$FAKE_DIR/args"
cat > "$FAKE_DIR/stdin"
printf '%s' "${ANTHROPIC_API_KEY-unset}" > "$FAKE_DIR/apikey"
printf '%s' "${CLAUDE_CODE_OAUTH_TOKEN-unset}" > "$FAKE_DIR/token"
if [[ -n "${FAKE_SLEEP:-}" ]]; then exec sleep "$FAKE_SLEEP"; fi
printf '%s' "${FAKE_STDERR:-}" >&2
printf '%s' "${FAKE_STDOUT:-}"
exit "${FAKE_EXIT:-0}"
`

func setup(t *testing.T, token string, opts ...llm.Option) (*llm.Runner, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(bin, []byte(fakeScript), 0o755))
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("FAKE_STDOUT", "  answer\n")

	tokens := &llm.MockTokenSource{
		ClaudeOAuthTokenFunc: func(context.Context) (string, error) { return token, nil },
	}
	return llm.New(tokens, append([]llm.Option{llm.WithBinary(bin)}, opts...)...), dir
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return string(data)
}

func TestRun(t *testing.T) {
	req := llm.Request{Model: "sonnet", Prompt: "hello prompt"}

	t.Run("returns trimmed stdout and passes args and stdin", func(t *testing.T) {
		r, dir := setup(t, "tok-123")

		out, err := r.Run(t.Context(), req)

		require.NoError(t, err)
		assert.Equal(t, "answer", out)
		assert.Equal(t, "-p\n--model\nsonnet\n", readFile(t, dir, "args"))
		assert.Equal(t, "hello prompt", readFile(t, dir, "stdin"))
	})

	t.Run("injects token and strips pre-existing credentials", func(t *testing.T) {
		r, dir := setup(t, "tok-123")
		t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "old-token")

		_, err := r.Run(t.Context(), req)

		require.NoError(t, err)
		assert.Equal(t, "tok-123", readFile(t, dir, "token"))
		assert.Equal(t, "unset", readFile(t, dir, "apikey"))
	})

	t.Run("api key set is refused as internal error", func(t *testing.T) {
		r, _ := setup(t, "tok-123")
		t.Setenv("ANTHROPIC_API_KEY", "sk-x")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, llm.ErrAnthropicEnvSet)
		require.ErrorIs(t, err, domain.ErrInternal)
	})

	t.Run("unsupported model is validation error", func(t *testing.T) {
		r, _ := setup(t, "tok-123")

		_, err := r.Run(t.Context(), llm.Request{Model: "gpt", Prompt: "x"})

		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("empty prompt is validation error", func(t *testing.T) {
		r, _ := setup(t, "tok-123")

		_, err := r.Run(t.Context(), llm.Request{Model: "opus"})

		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("token source error is propagated", func(t *testing.T) {
		boom := errors.New("boom")
		tokens := &llm.MockTokenSource{
			ClaudeOAuthTokenFunc: func(context.Context) (string, error) { return "", boom },
		}
		t.Setenv("ANTHROPIC_API_KEY", "")

		_, err := llm.New(tokens).Run(t.Context(), req)

		require.ErrorIs(t, err, boom)
	})

	t.Run("empty token is validation error", func(t *testing.T) {
		r, _ := setup(t, "")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Contains(t, err.Error(), "claude oauth token not configured")
	})

	t.Run("non-zero exit is upstream error with redacted stderr", func(t *testing.T) {
		r, _ := setup(t, "tok-123")
		t.Setenv("FAKE_EXIT", "3")
		t.Setenv("FAKE_STDERR", "auth failed for tok-123 end")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, domain.ErrUpstream)
		assert.Contains(t, err.Error(), "auth failed for [redacted] end")
		assert.NotContains(t, err.Error(), "tok-123")
	})

	t.Run("stderr is limited to last 400 bytes", func(t *testing.T) {
		r, _ := setup(t, "tok-123")
		t.Setenv("FAKE_EXIT", "1")
		long := strings.Repeat("aaaa", 100)
		t.Setenv("FAKE_STDERR", "HEAD"+long+"TAIL")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, domain.ErrUpstream)
		assert.NotContains(t, err.Error(), "HEAD")
		assert.Contains(t, err.Error(), "TAIL")
	})

	t.Run("timeout is upstream error", func(t *testing.T) {
		r, _ := setup(t, "tok-123", llm.WithTimeout(100*time.Millisecond))
		t.Setenv("FAKE_SLEEP", "5")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, domain.ErrUpstream)
	})

	t.Run("empty stdout is upstream error", func(t *testing.T) {
		r, _ := setup(t, "tok-123")
		t.Setenv("FAKE_STDOUT", " \n")

		_, err := r.Run(t.Context(), req)

		require.ErrorIs(t, err, domain.ErrUpstream)
	})
}
