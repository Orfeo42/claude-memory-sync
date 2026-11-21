package synthsettings

import (
	"context"
	"errors"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/flows/prompts"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/settings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errStore = errors.New("store down")

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func setValue(key, value string) settings.Setting {
	return settings.Setting{Key: key, Value: &value, Set: true, UpdatedAt: testTime, UpdatedBy: Actor}
}

func notFoundStore() *MockStore {
	return &MockStore{
		GetSettingFunc: func(_ context.Context, _ string) (settings.Setting, error) {
			return settings.Setting{}, domain.ErrNotFound
		},
		DeleteSettingFunc: func(_ context.Context, _, _ string) error { return nil },
		PutSettingFunc: func(_ context.Context, key string, req client.PutSettingRequest) (settings.Setting, error) {
			return setValue(key, req.Value), nil
		},
	}
}

func overrideStore(key, value string) *MockStore {
	store := notFoundStore()
	store.GetSettingFunc = func(_ context.Context, k string) (settings.Setting, error) {
		if k != key {
			return settings.Setting{}, domain.ErrNotFound
		}
		return setValue(key, value), nil
	}
	return store
}

func failingStore() *MockStore {
	return &MockStore{
		GetSettingFunc: func(_ context.Context, _ string) (settings.Setting, error) {
			return settings.Setting{}, errStore
		},
		ListSettingsFunc: func(_ context.Context) ([]settings.Setting, error) { return nil, errStore },
		PutSettingFunc: func(_ context.Context, _ string, _ client.PutSettingRequest) (settings.Setting, error) {
			return settings.Setting{}, errStore
		},
		DeleteSettingFunc: func(_ context.Context, _, _ string) error { return errStore },
		SettingHistoryFunc: func(_ context.Context, _ string, _ int) ([]settings.Version, error) {
			return nil, errStore
		},
	}
}

func TestDefaultSchedule(t *testing.T) {
	t.Run("each flow has its default", func(t *testing.T) {
		assert.Equal(t, "*/45 * * * *", DefaultSchedule(flows.Intake))
		assert.Equal(t, "0 3 * * *", DefaultSchedule(flows.DailySynth))
		assert.Equal(t, "0 4 * * 0", DefaultSchedule(flows.Taskmine))
		assert.Equal(t, "*/10 * * * *", DefaultSchedule(flows.ProposalApply))
	})

	t.Run("unknown flow is empty", func(t *testing.T) {
		assert.Empty(t, DefaultSchedule(flows.Name("nope")))
	})
}

func TestServicePrompt(t *testing.T) {
	def, err := prompts.Default("intake-judge")
	require.NoError(t, err)

	t.Run("override wins", func(t *testing.T) {
		svc := New(overrideStore("prompt.intake-judge", "custom"), "")
		got, err := svc.Prompt(context.Background(), "intake-judge")
		require.NoError(t, err)
		assert.Equal(t, "custom", got)
	})

	t.Run("default when not found", func(t *testing.T) {
		svc := New(notFoundStore(), "")
		got, err := svc.Prompt(context.Background(), "intake-judge")
		require.NoError(t, err)
		assert.Equal(t, def, got)
	})

	t.Run("unknown name is not found", func(t *testing.T) {
		svc := New(notFoundStore(), "")
		_, err := svc.Prompt(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("store error propagates", func(t *testing.T) {
		svc := New(failingStore(), "")
		_, err := svc.Prompt(context.Background(), "intake-judge")
		require.ErrorIs(t, err, errStore)
	})
}

func TestServicePrompts(t *testing.T) {
	names := prompts.Names()

	t.Run("one list call and no gets", func(t *testing.T) {
		store := notFoundStore()
		store.ListSettingsFunc = func(_ context.Context) ([]settings.Setting, error) {
			return []settings.Setting{setValue("prompt."+names[1], "x"), setValue("param.synth_model", "opus")}, nil
		}
		infos, err := New(store, "").Prompts(context.Background())
		require.NoError(t, err)
		assert.Len(t, store.ListSettingsCalls(), 1)
		assert.Empty(t, store.GetSettingCalls())
		require.Len(t, infos, len(names))
		assert.Equal(t, names[0], infos[0].Name)
		assert.False(t, infos[0].Overridden)
		assert.Equal(t, names[1], infos[1].Name)
		assert.True(t, infos[1].Overridden)
	})

	t.Run("overridden carries updated at", func(t *testing.T) {
		store := notFoundStore()
		store.ListSettingsFunc = func(_ context.Context) ([]settings.Setting, error) {
			return []settings.Setting{setValue("prompt."+names[1], "x")}, nil
		}
		infos, err := New(store, "").Prompts(context.Background())
		require.NoError(t, err)
		require.NotNil(t, infos[1].UpdatedAt)
		assert.Equal(t, testTime, *infos[1].UpdatedAt)
		assert.Nil(t, infos[0].UpdatedAt)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").Prompts(context.Background())
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceGetPrompt(t *testing.T) {
	def, err := prompts.Default("intake-judge")
	require.NoError(t, err)

	t.Run("overridden", func(t *testing.T) {
		got, err := New(overrideStore("prompt.intake-judge", "custom"), "").GetPrompt(context.Background(), "intake-judge")
		require.NoError(t, err)
		assert.Equal(t, "custom", got.Content)
		assert.Equal(t, def, got.Default)
		assert.True(t, got.Overridden)
		require.NotNil(t, got.UpdatedAt)
	})

	t.Run("default", func(t *testing.T) {
		got, err := New(notFoundStore(), "").GetPrompt(context.Background(), "intake-judge")
		require.NoError(t, err)
		assert.Equal(t, def, got.Content)
		assert.False(t, got.Overridden)
		assert.Nil(t, got.UpdatedAt)
	})

	t.Run("unknown name", func(t *testing.T) {
		_, err := New(notFoundStore(), "").GetPrompt(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceSetPrompt(t *testing.T) {
	t.Run("stores with ui actor", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").SetPrompt(context.Background(), "intake-judge", "new")
		require.NoError(t, err)
		assert.Equal(t, "new", got.Content)
		assert.True(t, got.Overridden)
		require.Len(t, store.PutSettingCalls(), 1)
		assert.Equal(t, "prompt.intake-judge", store.PutSettingCalls()[0].Key)
		assert.Equal(t, Actor, store.PutSettingCalls()[0].Req.Actor)
	})

	t.Run("empty content is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetPrompt(context.Background(), "intake-judge", "")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("too long content is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetPrompt(context.Background(), "intake-judge", string(make([]byte, 200001)))
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("max length accepted", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetPrompt(context.Background(), "intake-judge", string(make([]byte, 200000)))
		require.NoError(t, err)
	})

	t.Run("unknown name", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetPrompt(context.Background(), "nope", "x")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").SetPrompt(context.Background(), "intake-judge", "x")
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceResetPrompt(t *testing.T) {
	t.Run("deletes and returns default", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").ResetPrompt(context.Background(), "intake-judge")
		require.NoError(t, err)
		assert.False(t, got.Overridden)
		assert.Equal(t, got.Default, got.Content)
		require.Len(t, store.DeleteSettingCalls(), 1)
		assert.Equal(t, "prompt.intake-judge", store.DeleteSettingCalls()[0].Key)
		assert.Equal(t, Actor, store.DeleteSettingCalls()[0].Actor)
	})

	t.Run("not found on delete is ignored", func(t *testing.T) {
		store := notFoundStore()
		store.DeleteSettingFunc = func(_ context.Context, _, _ string) error { return domain.ErrNotFound }
		_, err := New(store, "").ResetPrompt(context.Background(), "intake-judge")
		require.NoError(t, err)
	})

	t.Run("unknown name", func(t *testing.T) {
		_, err := New(notFoundStore(), "").ResetPrompt(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").ResetPrompt(context.Background(), "intake-judge")
		require.ErrorIs(t, err, errStore)
	})
}

func TestServicePromptHistory(t *testing.T) {
	t.Run("maps versions", func(t *testing.T) {
		content := "v1"
		store := notFoundStore()
		store.SettingHistoryFunc = func(_ context.Context, _ string, _ int) ([]settings.Version, error) {
			return []settings.Version{{Value: &content, CreatedAt: testTime, CreatedBy: "ui"}, {Deleted: true, CreatedAt: testTime, CreatedBy: "ui"}}, nil
		}
		got, err := New(store, "").PromptHistory(context.Background(), "intake-judge", 5)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, &content, got[0].Content)
		assert.True(t, got[1].Deleted)
		assert.Nil(t, got[1].Content)
		assert.Equal(t, "prompt.intake-judge", store.SettingHistoryCalls()[0].Key)
		assert.Equal(t, 5, store.SettingHistoryCalls()[0].Limit)
	})

	t.Run("unknown name", func(t *testing.T) {
		_, err := New(notFoundStore(), "").PromptHistory(context.Background(), "nope", 5)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").PromptHistory(context.Background(), "intake-judge", 5)
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceParam(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		got, err := New(overrideStore("param.synth_model", "opus"), "").Param(context.Background(), flows.ParamSynthModel)
		require.NoError(t, err)
		assert.Equal(t, "opus", got)
	})

	t.Run("default when not found", func(t *testing.T) {
		got, err := New(notFoundStore(), "").Param(context.Background(), flows.ParamHotcacheMaxBytes)
		require.NoError(t, err)
		assert.Equal(t, "4000", got)
	})

	t.Run("unknown key is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").Param(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").Param(context.Background(), flows.ParamSynthModel)
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceParams(t *testing.T) {
	t.Run("one list call in catalog order with overrides", func(t *testing.T) {
		store := notFoundStore()
		store.ListSettingsFunc = func(_ context.Context) ([]settings.Setting, error) {
			return []settings.Setting{setValue("param.fidelity_model", "opus")}, nil
		}
		got, err := New(store, "").Params(context.Background())
		require.NoError(t, err)
		assert.Len(t, store.ListSettingsCalls(), 1)
		assert.Empty(t, store.GetSettingCalls())
		require.Len(t, got, 4)
		assert.Equal(t, flows.ParamSynthModel, got[0].Key)
		assert.Equal(t, flows.ParamFidelityModel, got[1].Key)
		assert.Equal(t, flows.ParamStalenessEnabled, got[2].Key)
		assert.Equal(t, flows.ParamHotcacheMaxBytes, got[3].Key)
		assert.Equal(t, "opus", got[1].Value)
		assert.Equal(t, "haiku", got[1].Default)
		assert.True(t, got[1].Overridden)
		assert.Equal(t, "sonnet", got[0].Value)
		assert.False(t, got[0].Overridden)
		assert.Equal(t, KindBool, got[2].Kind)
		assert.Equal(t, KindInt, got[3].Kind)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").Params(context.Background())
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceSetParam(t *testing.T) {
	t.Run("model sonnet accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamSynthModel, "sonnet")
		require.NoError(t, err)
		assert.Equal(t, "sonnet", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("model opus accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamSynthModel, "opus")
		require.NoError(t, err)
		assert.Equal(t, "opus", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("model haiku accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamSynthModel, "haiku")
		require.NoError(t, err)
		assert.Equal(t, "haiku", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("model unknown value rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamSynthModel, "gpt")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("bool true accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamStalenessEnabled, "true")
		require.NoError(t, err)
		assert.Equal(t, "true", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("bool false accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamStalenessEnabled, "false")
		require.NoError(t, err)
		assert.Equal(t, "false", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("bool invalid value", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamStalenessEnabled, "yes")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int minimum accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "1")
		require.NoError(t, err)
		assert.Equal(t, "1", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("int typical accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "4000")
		require.NoError(t, err)
		assert.Equal(t, "4000", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("int maximum accepted", func(t *testing.T) {
		got, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "1000000")
		require.NoError(t, err)
		assert.Equal(t, "1000000", got.Value)
		assert.True(t, got.Overridden)
	})

	t.Run("int zero rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "0")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int negative rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "-1")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int plus sign rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "+5")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int above maximum rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "1000001")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int non numeric rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "abc")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int empty rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("int decimal rejected", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), flows.ParamHotcacheMaxBytes, "1.5")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("unknown key is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetParam(context.Background(), "nope", "x")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("invalid value is not stored", func(t *testing.T) {
		store := notFoundStore()
		_, err := New(store, "").SetParam(context.Background(), flows.ParamSynthModel, "gpt")
		require.Error(t, err)
		assert.Empty(t, store.PutSettingCalls())
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").SetParam(context.Background(), flows.ParamSynthModel, "opus")
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceResetParam(t *testing.T) {
	t.Run("deletes and returns default", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").ResetParam(context.Background(), flows.ParamSynthModel)
		require.NoError(t, err)
		assert.Equal(t, "sonnet", got.Value)
		assert.False(t, got.Overridden)
		assert.Equal(t, "param.synth_model", store.DeleteSettingCalls()[0].Key)
	})

	t.Run("unknown key is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").ResetParam(context.Background(), "nope")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").ResetParam(context.Background(), flows.ParamSynthModel)
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceSchedule(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		got, err := New(overrideStore("schedule.intake", "5 * * * *"), "").Schedule(context.Background(), flows.Intake)
		require.NoError(t, err)
		assert.Equal(t, "5 * * * *", got)
	})

	t.Run("default when not found", func(t *testing.T) {
		got, err := New(notFoundStore(), "").Schedule(context.Background(), flows.DailySynth)
		require.NoError(t, err)
		assert.Equal(t, "0 3 * * *", got)
	})

	t.Run("unknown flow is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").Schedule(context.Background(), flows.Name("nope"))
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").Schedule(context.Background(), flows.Intake)
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceSetSchedule(t *testing.T) {
	t.Run("valid cron stored and returned", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").SetSchedule(context.Background(), flows.Taskmine, "30 2 * * 1")
		require.NoError(t, err)
		assert.Equal(t, "30 2 * * 1", got)
		assert.Equal(t, "schedule.taskmine", store.PutSettingCalls()[0].Key)
	})

	t.Run("invalid cron is validation", func(t *testing.T) {
		store := notFoundStore()
		_, err := New(store, "").SetSchedule(context.Background(), flows.Taskmine, "not cron")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, store.PutSettingCalls())
	})

	t.Run("unknown flow is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").SetSchedule(context.Background(), flows.Name("nope"), "* * * * *")
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").SetSchedule(context.Background(), flows.Intake, "* * * * *")
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceResetSchedule(t *testing.T) {
	t.Run("deletes and returns default", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").ResetSchedule(context.Background(), flows.ProposalApply)
		require.NoError(t, err)
		assert.Equal(t, "*/10 * * * *", got)
		assert.Equal(t, "schedule.proposal-apply", store.DeleteSettingCalls()[0].Key)
	})

	t.Run("unknown flow is validation", func(t *testing.T) {
		_, err := New(notFoundStore(), "").ResetSchedule(context.Background(), flows.Name("nope"))
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").ResetSchedule(context.Background(), flows.Intake)
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceSetToken(t *testing.T) {
	t.Run("trims and stores", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").SetToken(context.Background(), "  tok-secret \n")
		require.NoError(t, err)
		assert.True(t, got.Set)
		require.NotNil(t, got.UpdatedAt)
		assert.Equal(t, "secret.claude-oauth-token", store.PutSettingCalls()[0].Key)
		assert.Equal(t, "tok-secret", store.PutSettingCalls()[0].Req.Value)
		assert.Equal(t, Actor, store.PutSettingCalls()[0].Req.Actor)
	})

	t.Run("empty is validation", func(t *testing.T) {
		store := notFoundStore()
		_, err := New(store, "").SetToken(context.Background(), "   ")
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, store.PutSettingCalls())
	})

	t.Run("too long is validation and hides the value", func(t *testing.T) {
		long := "sekrit" + string(make([]byte, 4096))
		_, err := New(notFoundStore(), "").SetToken(context.Background(), long)
		require.ErrorIs(t, err, domain.ErrValidation)
		assert.NotContains(t, err.Error(), "sekrit")
	})

	t.Run("store error propagates without the value", func(t *testing.T) {
		_, err := New(failingStore(), "").SetToken(context.Background(), "sekrit-value")
		require.ErrorIs(t, err, errStore)
		assert.NotContains(t, err.Error(), "sekrit-value")
	})
}

func TestServiceClearToken(t *testing.T) {
	t.Run("deletes and reports unset", func(t *testing.T) {
		store := notFoundStore()
		got, err := New(store, "").ClearToken(context.Background())
		require.NoError(t, err)
		assert.False(t, got.Set)
		assert.Equal(t, "secret.claude-oauth-token", store.DeleteSettingCalls()[0].Key)
	})

	t.Run("falls back to env status", func(t *testing.T) {
		got, err := New(notFoundStore(), "env-tok").ClearToken(context.Background())
		require.NoError(t, err)
		assert.True(t, got.Set)
		assert.Nil(t, got.UpdatedAt)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").ClearToken(context.Background())
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceTokenStatus(t *testing.T) {
	t.Run("db row", func(t *testing.T) {
		got, err := New(overrideStore("secret.claude-oauth-token", "tok"), "env").TokenStatus(context.Background())
		require.NoError(t, err)
		assert.True(t, got.Set)
		require.NotNil(t, got.UpdatedAt)
		assert.Equal(t, testTime, *got.UpdatedAt)
	})

	t.Run("env fallback", func(t *testing.T) {
		got, err := New(notFoundStore(), "env").TokenStatus(context.Background())
		require.NoError(t, err)
		assert.True(t, got.Set)
		assert.Nil(t, got.UpdatedAt)
	})

	t.Run("nothing configured", func(t *testing.T) {
		got, err := New(notFoundStore(), "").TokenStatus(context.Background())
		require.NoError(t, err)
		assert.False(t, got.Set)
		assert.Nil(t, got.UpdatedAt)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "").TokenStatus(context.Background())
		require.ErrorIs(t, err, errStore)
	})
}

func TestServiceClaudeOAuthToken(t *testing.T) {
	t.Run("db value wins over env", func(t *testing.T) {
		got, err := New(overrideStore("secret.claude-oauth-token", "db-tok"), "env-tok").ClaudeOAuthToken(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "db-tok", got)
	})

	t.Run("env when db empty", func(t *testing.T) {
		got, err := New(notFoundStore(), "env-tok").ClaudeOAuthToken(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "env-tok", got)
	})

	t.Run("empty when nothing configured", func(t *testing.T) {
		got, err := New(notFoundStore(), "").ClaudeOAuthToken(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("store error propagates", func(t *testing.T) {
		_, err := New(failingStore(), "env-tok").ClaudeOAuthToken(context.Background())
		require.ErrorIs(t, err, errStore)
	})
}
