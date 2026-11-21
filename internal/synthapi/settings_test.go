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

const secretToken = "sk-ant-oat01-SECRET"

func TestGetSettings(t *testing.T) {
	t.Run("returns params and token status", func(t *testing.T) {
		st := &MockSettings{
			ParamsFunc: func(_ context.Context) ([]synthsettings.Param, error) {
				return []synthsettings.Param{{Key: "synth_model", Value: "sonnet", Default: "sonnet", Kind: synthsettings.KindModel}}, nil
			},
			TokenStatusFunc: func(_ context.Context) (synthsettings.TokenStatus, error) {
				return synthsettings.TokenStatus{Set: true}, nil
			},
		}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/settings", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"key":"synth_model"`)
		assert.Contains(t, rec.Body.String(), `"token":{"set":true,"updated_at":null}`)
	})

	t.Run("params error is 502 when upstream", func(t *testing.T) {
		st := &MockSettings{ParamsFunc: func(_ context.Context) ([]synthsettings.Param, error) {
			return nil, domain.Error(domain.ErrUpstream, "down")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/settings", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("token status error is 500", func(t *testing.T) {
		st := &MockSettings{
			ParamsFunc: func(_ context.Context) ([]synthsettings.Param, error) { return nil, nil },
			TokenStatusFunc: func(_ context.Context) (synthsettings.TokenStatus, error) {
				return synthsettings.TokenStatus{}, assert.AnError
			},
		}
		rec := do(t, New(Deps{Settings: st}), http.MethodGet, "/api/settings", "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestPutParam(t *testing.T) {
	t.Run("sets value", func(t *testing.T) {
		st := &MockSettings{SetParamFunc: func(_ context.Context, key, value string) (synthsettings.Param, error) {
			return synthsettings.Param{Key: key, Value: value, Overridden: true}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/settings/params/synth_model", `{"value":"opus"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "opus", st.SetParamCalls()[0].Value)
		assert.Equal(t, "synth_model", st.SetParamCalls()[0].Key)
	})

	t.Run("invalid value is 422", func(t *testing.T) {
		st := &MockSettings{SetParamFunc: func(_ context.Context, _, _ string) (synthsettings.Param, error) {
			return synthsettings.Param{}, domain.Error(domain.ErrValidation, "bad model")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/settings/params/synth_model", `{"value":"gpt"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("unknown key is 404", func(t *testing.T) {
		st := &MockSettings{SetParamFunc: func(_ context.Context, _, _ string) (synthsettings.Param, error) {
			return synthsettings.Param{}, domain.Error(domain.ErrNotFound, "no param")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/settings/params/zzz", `{"value":"1"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestResetParam(t *testing.T) {
	t.Run("resets", func(t *testing.T) {
		st := &MockSettings{ResetParamFunc: func(_ context.Context, key string) (synthsettings.Param, error) {
			return synthsettings.Param{Key: key}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/settings/params/synth_model", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "synth_model", st.ResetParamCalls()[0].Key)
	})

	t.Run("unknown key is 404", func(t *testing.T) {
		st := &MockSettings{ResetParamFunc: func(_ context.Context, _ string) (synthsettings.Param, error) {
			return synthsettings.Param{}, domain.Error(domain.ErrNotFound, "no param")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/settings/params/zzz", "")
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestPutToken(t *testing.T) {
	t.Run("stores token and never echoes it", func(t *testing.T) {
		st := &MockSettings{SetTokenFunc: func(_ context.Context, _ string) (synthsettings.TokenStatus, error) {
			return synthsettings.TokenStatus{Set: true}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/settings/token", `{"token":"`+secretToken+`"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, secretToken, st.SetTokenCalls()[0].Token)
		assert.NotContains(t, rec.Body.String(), secretToken)
		assert.Contains(t, rec.Body.String(), `"set":true`)
	})

	t.Run("failure does not leak the token", func(t *testing.T) {
		st := &MockSettings{SetTokenFunc: func(_ context.Context, _ string) (synthsettings.TokenStatus, error) {
			return synthsettings.TokenStatus{}, domain.Error(domain.ErrUpstream, "store down")
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodPut, "/api/settings/token", `{"token":"`+secretToken+`"}`)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
		assert.NotContains(t, rec.Body.String(), secretToken)
	})

	t.Run("empty token is 422", func(t *testing.T) {
		rec := do(t, New(Deps{Settings: &MockSettings{}}), http.MethodPut, "/api/settings/token", `{"token":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestClearToken(t *testing.T) {
	t.Run("clears and returns status", func(t *testing.T) {
		st := &MockSettings{ClearTokenFunc: func(_ context.Context) (synthsettings.TokenStatus, error) {
			return synthsettings.TokenStatus{Set: false}, nil
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/settings/token", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"set":false,"updated_at":null}`, rec.Body.String())
	})

	t.Run("error is 500", func(t *testing.T) {
		st := &MockSettings{ClearTokenFunc: func(_ context.Context) (synthsettings.TokenStatus, error) {
			return synthsettings.TokenStatus{}, assert.AnError
		}}
		rec := do(t, New(Deps{Settings: st}), http.MethodDelete, "/api/settings/token", "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
