package domain_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamErrorSentinelClassification(t *testing.T) {
	t.Run("404 classifies as not found", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusNotFound}

		require.ErrorIs(t, err, domain.ErrNotFound)
		assert.False(t, err.IsOurFault())
	})

	t.Run("409 classifies as conflict", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusConflict}

		require.ErrorIs(t, err, domain.ErrConflict)
		assert.False(t, err.IsOurFault())
	})

	t.Run("401 classifies as internal and is our fault", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusUnauthorized}

		require.ErrorIs(t, err, domain.ErrInternal)
		assert.True(t, err.IsOurFault())
	})

	t.Run("403 classifies as internal and is our fault", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusForbidden}

		require.ErrorIs(t, err, domain.ErrInternal)
		assert.True(t, err.IsOurFault())
	})

	t.Run("500 and above classifies as upstream", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusInternalServerError}

		require.ErrorIs(t, err, domain.ErrUpstream)
		assert.False(t, err.IsOurFault())
	})

	t.Run("503 classifies as upstream", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusServiceUnavailable}

		require.ErrorIs(t, err, domain.ErrUpstream)
		assert.False(t, err.IsOurFault())
	})

	t.Run("400 and above below 500 classifies as validation", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusBadRequest}

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.False(t, err.IsOurFault())
	})

	t.Run("422 classifies as validation", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusUnprocessableEntity}

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.False(t, err.IsOurFault())
	})

	t.Run("below 400 classifies as internal", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusTeapot - 100}

		require.ErrorIs(t, err, domain.ErrInternal)
		assert.True(t, err.IsOurFault())
	})

	t.Run("survives a wrap error layer", func(t *testing.T) {
		upstreamErr := &domain.UpstreamError{API: "billing", StatusCode: http.StatusNotFound}
		wrapped := domain.WrapError(upstreamErr)

		var target *domain.UpstreamError
		require.ErrorAs(t, wrapped, &target)
		assert.Equal(t, "billing", target.API)
		require.ErrorIs(t, wrapped, domain.ErrNotFound)
	})
}

func TestUpstreamErrorStatusCodeFromError(t *testing.T) {
	t.Run("maps through StatusCodeFromError", func(t *testing.T) {
		err := &domain.UpstreamError{API: "billing", StatusCode: http.StatusInternalServerError}

		assert.Equal(t, http.StatusBadGateway, domain.StatusCodeFromError(err))
	})
}

func TestUpstreamUnreachableError(t *testing.T) {
	t.Run("Is matches ErrUpstream", func(t *testing.T) {
		err := &domain.UpstreamUnreachableError{API: "billing", Cause: errors.New("dial tcp: timeout")}

		require.ErrorIs(t, err, domain.ErrUpstream)
	})

	t.Run("root cause preserved through fmt errorf wrap", func(t *testing.T) {
		cause := errors.New("dial tcp: timeout")
		err := &domain.UpstreamUnreachableError{API: "billing", Cause: cause}
		wrapped := fmt.Errorf("calling billing: %w", err)

		require.ErrorIs(t, wrapped, cause)
		require.ErrorIs(t, wrapped, domain.ErrUpstream)
	})

	t.Run("status code resolves to bad gateway", func(t *testing.T) {
		err := &domain.UpstreamUnreachableError{API: "billing", Cause: errors.New("dial tcp: timeout")}

		assert.Equal(t, http.StatusBadGateway, domain.StatusCodeFromError(err))
	})
}
