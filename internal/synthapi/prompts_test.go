package synthapi

import (
	"context"
	"net/http"
	"testing"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/synthsettings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPrompts(t *testing.T) {
	t.Run("wraps list", func(t *testing.T) {
		st := &MockSettings{PromptsFunc: func(_ context.Context) ([]synthsettings.PromptInfo, error) {
			return []synthsettings.PromptInfo{{Name: "a", Overridden: true}}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"prompts":[{"name":"a","overridden":true,"updated_at":null}]}`, rec.Body.String())
	})

	t.Run("empty is an empty list", func(t *testing.T) {
		st := &MockSettings{PromptsFunc: func(_ context.Context) ([]synthsettings.PromptInfo, error) { return nil, nil }}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts", "")
		assert.JSONEq(t, `{"prompts":[]}`, rec.Body.String())
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		st := &MockSettings{PromptsFunc: func(_ context.Context) ([]synthsettings.PromptInfo, error) {
			return nil, domain.Error(domain.ErrUpstream, "down")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestGetPrompt(t *testing.T) {
	t.Run("returns prompt", func(t *testing.T) {
		st := &MockSettings{GetPromptFunc: func(_ context.Context, name string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{Name: name, Content: "c", Default: "d"}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts/intake-judge", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"name":"intake-judge","content":"c","default":"d","overridden":false,"updated_at":null}`, rec.Body.String())
	})

	t.Run("unknown prompt is 404", func(t *testing.T) {
		st := &MockSettings{GetPromptFunc: func(_ context.Context, _ string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{}, domain.Error(domain.ErrNotFound, "no prompt")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts/nope", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestPutPrompt(t *testing.T) {
	t.Run("stores content", func(t *testing.T) {
		st := &MockSettings{SetPromptFunc: func(_ context.Context, name, content string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{Name: name, Content: content, Overridden: true}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/prompts/a", `{"content":"new"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "new", st.SetPromptCalls()[0].Content)
	})

	t.Run("empty content is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Settings: &MockSettings{}}), http.MethodPut, "/api/prompts/a", `{"content":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("validation error from settings is 422", func(t *testing.T) {
		st := &MockSettings{SetPromptFunc: func(_ context.Context, _, _ string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{}, domain.Error(domain.ErrValidation, "bad")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/prompts/a", `{"content":"x"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestResetPrompt(t *testing.T) {
	t.Run("resets to default", func(t *testing.T) {
		st := &MockSettings{ResetPromptFunc: func(_ context.Context, name string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{Name: name}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/prompts/a", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "a", st.ResetPromptCalls()[0].Name)
	})

	t.Run("unknown prompt is 404", func(t *testing.T) {
		st := &MockSettings{ResetPromptFunc: func(_ context.Context, _ string) (synthsettings.Prompt, error) {
			return synthsettings.Prompt{}, domain.Error(domain.ErrNotFound, "no prompt")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/prompts/x", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestPromptHistory(t *testing.T) {
	t.Run("returns versions with default limit", func(t *testing.T) {
		content := "old"
		st := &MockSettings{PromptHistoryFunc: func(_ context.Context, _ string, _ int) ([]synthsettings.PromptVersion, error) {
			return []synthsettings.PromptVersion{{Content: &content, CreatedAt: t0, CreatedBy: "ui"}}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts/a/history", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 50, st.PromptHistoryCalls()[0].Limit)
		assert.Contains(t, rec.Body.String(), `"versions":[{"content":"old"`)
	})

	t.Run("empty is an empty list", func(t *testing.T) {
		st := &MockSettings{PromptHistoryFunc: func(_ context.Context, _ string, _ int) ([]synthsettings.PromptVersion, error) {
			return nil, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/prompts/a/history?limit=3", "")

		assert.JSONEq(t, `{"versions":[]}`, rec.Body.String())
		assert.Equal(t, 3, st.PromptHistoryCalls()[0].Limit)
	})

	t.Run("limit zero is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Settings: &MockSettings{}}), http.MethodGet, "/api/prompts/a/history?limit=0", "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}
