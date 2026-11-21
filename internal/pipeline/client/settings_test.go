package client

import (
	"net/http"
	"testing"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const settingJSON = `{"key":"param.a","value":"v","set":true,"updated_at":"2026-01-01T00:00:00Z","updated_by":"me"}`

func TestListSettings(t *testing.T) {
	t.Run("gets and decodes the wrapped list", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"settings":[`+settingJSON+`]}`)

		got, err := c.ListSettings(t.Context())
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "param.a", got[0].Key)
		assert.Equal(t, http.MethodGet, rec.method)
		assert.Equal(t, "/v1/settings", rec.path)
		assert.Equal(t, "Bearer "+testToken, rec.auth)
	})

	t.Run("null list becomes empty slice", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusOK, `{"settings":null}`)
		got, err := c.ListSettings(t.Context())
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestGetSetting(t *testing.T) {
	t.Run("gets the row", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, settingJSON)

		got, err := c.GetSetting(t.Context(), "param.a")
		require.NoError(t, err)
		assert.Equal(t, "v", *got.Value)
		assert.Equal(t, "/v1/settings/param.a", rec.path)
	})

	t.Run("404 maps to not found", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusNotFound, `{"detail":"setting not found"}`)
		_, err := c.GetSetting(t.Context(), "param.a")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestPutSetting(t *testing.T) {
	t.Run("puts the body", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, settingJSON)

		got, err := c.PutSetting(t.Context(), "param.a", PutSettingRequest{Value: "v", Actor: "me"})
		require.NoError(t, err)
		assert.Equal(t, "param.a", got.Key)
		assert.Equal(t, http.MethodPut, rec.method)
		assert.Equal(t, "/v1/settings/param.a", rec.path)
		assert.Equal(t, "v", rec.body["value"])
		assert.Equal(t, "me", rec.body["actor"])
	})

	t.Run("403 maps to an error", func(t *testing.T) {
		c, _ := fakeServer(t, http.StatusForbidden, `{"detail":"forbidden"}`)
		_, err := c.PutSetting(t.Context(), "param.a", PutSettingRequest{Value: "v", Actor: "me"})
		require.Error(t, err)
	})
}

func TestDeleteSetting(t *testing.T) {
	t.Run("sends actor as query", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusNoContent, "")

		require.NoError(t, c.DeleteSetting(t.Context(), "param.a", "me"))
		assert.Equal(t, http.MethodDelete, rec.method)
		assert.Equal(t, "/v1/settings/param.a", rec.path)
		assert.Equal(t, "actor=me", rec.query)
	})
}

func TestSettingHistory(t *testing.T) {
	t.Run("sends limit and decodes versions", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"versions":[{"value":"v","deleted":false,"created_at":"2026-01-01T00:00:00Z","created_by":"me"}]}`)

		got, err := c.SettingHistory(t.Context(), "param.a", 5)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "me", got[0].CreatedBy)
		assert.Equal(t, "/v1/settings/param.a/history", rec.path)
		assert.Equal(t, "limit=5", rec.query)
	})

	t.Run("zero limit sends no query", func(t *testing.T) {
		c, rec := fakeServer(t, http.StatusOK, `{"versions":null}`)

		got, err := c.SettingHistory(t.Context(), "param.a", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, rec.query)
	})
}
