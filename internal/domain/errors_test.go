package domain_test

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError(t *testing.T) {
	t.Run("returns nil on nil input", func(t *testing.T) {
		require.NoError(t, domain.Error(nil, "some message"))
	})

	t.Run("preserves identity via errors.Is", func(t *testing.T) {
		wrapped := domain.Error(domain.ErrNotFound, "record lookup failed")

		require.ErrorIs(t, wrapped, domain.ErrNotFound)
	})

	t.Run("origin attr contains the test file name", func(t *testing.T) {
		wrapped := domain.Error(domain.ErrNotFound, "record lookup failed")

		attrs := domain.ErrorAttrs(wrapped)
		origins := findOriginAttr(t, attrs)
		assertAnyContains(t, origins, "errors_test.go")
	})

	t.Run("sets user facing message", func(t *testing.T) {
		wrapped := domain.Error(domain.ErrValidation, "email is required")

		assert.Equal(t, "email is required", domain.UserMessage(wrapped))
	})
}

func TestWrapError(t *testing.T) {
	t.Run("returns nil on nil input", func(t *testing.T) {
		require.NoError(t, domain.WrapError(nil))
	})

	t.Run("preserves identity via errors.Is", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrUpstream, slog.String("api", "billing"))

		require.ErrorIs(t, wrapped, domain.ErrUpstream)
	})

	t.Run("origin attr contains the test file name", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrUpstream)

		attrs := domain.ErrorAttrs(wrapped)
		origins := findOriginAttr(t, attrs)
		assertAnyContains(t, origins, "errors_test.go")
	})

	t.Run("does not set a user facing message", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrInternal)

		assert.Equal(t, "an internal error occurred", domain.UserMessage(wrapped))
	})
}

func findOriginAttr(t *testing.T, attrs []slog.Attr) []string {
	t.Helper()

	for _, a := range attrs {
		if a.Key != "origin" {
			continue
		}
		origins, ok := a.Value.Any().([]string)
		if !ok {
			t.Fatal("origin attr value is not a []string")
		}
		return origins
	}
	t.Fatal("no origin attr found")
	return nil
}

func assertAnyContains(t *testing.T, values []string, substr string) {
	t.Helper()

	for _, v := range values {
		if strings.Contains(v, substr) {
			return
		}
	}
	t.Fatalf("no value in %v contains %q", values, substr)
}

func TestErrorAttrs(t *testing.T) {
	t.Run("collects attrs from a single wrap", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrValidation, slog.String("field", "email"))

		attrs := domain.ErrorAttrs(wrapped)

		require.True(t, hasAttr(attrs, "field", "email"))
	})

	t.Run("collects attrs across nested wraps outer to inner", func(t *testing.T) {
		inner := domain.WrapError(domain.ErrValidation, slog.String("layer", "inner"))
		outer := domain.WrapError(inner, slog.String("layer", "outer"))

		attrs := domain.ErrorAttrs(outer)

		require.True(t, hasAttr(attrs, "layer", "outer"))
		require.True(t, hasAttr(attrs, "layer", "inner"))
	})

	t.Run("survives an interposed fmt errorf layer", func(t *testing.T) {
		inner := domain.WrapError(domain.ErrValidation, slog.String("field", "email"))
		wrapped := fmt.Errorf("handler failed: %w", inner)

		attrs := domain.ErrorAttrs(wrapped)

		require.True(t, hasAttr(attrs, "field", "email"))
	})

	t.Run("plain error has no attrs", func(t *testing.T) {
		attrs := domain.ErrorAttrs(errors.New("boom"))

		assert.Empty(t, attrs)
	})

	t.Run("origin order is innermost first", func(t *testing.T) {
		inner := domain.WrapError(domain.ErrValidation)
		outer := domain.WrapError(inner)

		origins := findOriginAttr(t, domain.ErrorAttrs(outer))

		require.Len(t, origins, 2)
		assert.Contains(t, origins[0], "errors_test.go")
		assert.Contains(t, origins[1], "errors_test.go")
	})
}

func hasAttr(attrs []slog.Attr, key, value string) bool {
	for _, a := range attrs {
		if a.Key == key && a.Value.String() == value {
			return true
		}
	}
	return false
}

func TestStatusCodeFromError(t *testing.T) {
	t.Run("not found maps to 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, domain.StatusCodeFromError(domain.ErrNotFound))
	})

	t.Run("unauthorized maps to 401", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, domain.StatusCodeFromError(domain.ErrUnauthorized))
	})

	t.Run("conflict maps to 409", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, domain.StatusCodeFromError(domain.ErrConflict))
	})

	t.Run("upstream maps to 502", func(t *testing.T) {
		assert.Equal(t, http.StatusBadGateway, domain.StatusCodeFromError(domain.ErrUpstream))
	})

	t.Run("validation maps to 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, domain.StatusCodeFromError(domain.ErrValidation))
	})

	t.Run("unknown error maps to 500", func(t *testing.T) {
		assert.Equal(t, http.StatusInternalServerError, domain.StatusCodeFromError(errors.New("boom")))
	})

	t.Run("wrapped sentinel resolves through the chain", func(t *testing.T) {
		wrapped := fmt.Errorf("layer: %w", domain.Error(domain.ErrConflict, "duplicate entry"))

		assert.Equal(t, http.StatusConflict, domain.StatusCodeFromError(wrapped))
	})
}

func TestUserMessage(t *testing.T) {
	t.Run("returns explicit message when set", func(t *testing.T) {
		wrapped := domain.Error(domain.ErrValidation, "email is required")

		assert.Equal(t, "email is required", domain.UserMessage(wrapped))
	})

	t.Run("returns generic message by status code when unset", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrNotFound)

		assert.Equal(t, "the requested resource was not found", domain.UserMessage(wrapped))
	})

	t.Run("does not leak wrapped internal detail", func(t *testing.T) {
		internal := domain.WrapError(domain.ErrInternal, slog.String("dsn", "postgres://secret"))

		msg := domain.UserMessage(internal)

		assert.NotContains(t, msg, "postgres")
		assert.Equal(t, "an internal error occurred", msg)
	})

	t.Run("outermost explicit message wins over inner one", func(t *testing.T) {
		inner := domain.Error(domain.ErrValidation, "inner message")
		outer := domain.Error(inner, "outer message")

		assert.Equal(t, "outer message", domain.UserMessage(outer))
	})
}

func TestUserMessageOr(t *testing.T) {
	t.Run("returns explicit message when set", func(t *testing.T) {
		wrapped := domain.Error(domain.ErrValidation, "email is required")

		assert.Equal(t, "email is required", domain.UserMessageOr(wrapped, "fallback"))
	})

	t.Run("returns fallback when no explicit message is set", func(t *testing.T) {
		wrapped := domain.WrapError(domain.ErrInternal)

		assert.Equal(t, "fallback", domain.UserMessageOr(wrapped, "fallback"))
	})

	t.Run("does not leak wrapped internal detail", func(t *testing.T) {
		internal := domain.WrapError(domain.ErrInternal, slog.String("dsn", "postgres://secret"))

		msg := domain.UserMessageOr(internal, "something went wrong")

		assert.NotContains(t, msg, "postgres")
		assert.Equal(t, "something went wrong", msg)
	})
}
