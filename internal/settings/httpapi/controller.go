package httpapi

import (
	"context"
	"net/http"

	"claude-memory-sync/internal/pipeline/httpapi"
	"claude-memory-sync/internal/settings"

	"github.com/danielgtaylor/huma/v2"
)

const (
	settingsPath = "/v1/settings"
	settingPath  = "/v1/settings/{key}"
	historyPath  = "/v1/settings/{key}/history"
)

type controller struct {
	svc *settings.Service
}

type emptyInput struct{}

type emptyOutput struct{}

type keyInput struct {
	Key string `path:"key" pattern:"^(prompt|param|schedule|secret)\\.[a-z0-9][a-z0-9_-]{0,63}$" maxLength:"70"`
}

type listOutput struct {
	Body struct {
		Settings []settings.Setting `json:"settings"`
	}
}

type settingOutput struct {
	Body settings.Setting
}

type putInput struct {
	Key  string `path:"key" pattern:"^(prompt|param|schedule|secret)\\.[a-z0-9][a-z0-9_-]{0,63}$" maxLength:"70"`
	Body struct {
		Value string `json:"value" maxLength:"200000"`
		Actor string `json:"actor" minLength:"1" maxLength:"200"`
	}
}

type deleteInput struct {
	Key   string `path:"key" pattern:"^(prompt|param|schedule|secret)\\.[a-z0-9][a-z0-9_-]{0,63}$" maxLength:"70"`
	Actor string `query:"actor" required:"true" minLength:"1" maxLength:"200"`
}

type historyInput struct {
	Key   string `path:"key" pattern:"^(prompt|param|schedule|secret)\\.[a-z0-9][a-z0-9_-]{0,63}$" maxLength:"70"`
	Limit int    `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type historyOutput struct {
	Body struct {
		Versions []settings.Version `json:"versions"`
	}
}

func Register(svc *settings.Service) httpapi.Registrar {
	return func(api huma.API) {
		c := &controller{svc: svc}
		huma.Register(api, httpapi.Operation("list-settings", http.MethodGet, settingsPath, httpapi.RoleWriter, http.StatusOK), c.list)
		huma.Register(api, httpapi.Operation("get-setting", http.MethodGet, settingPath, httpapi.RoleWriter, http.StatusOK), c.get)
		huma.Register(api, httpapi.Operation("put-setting", http.MethodPut, settingPath, httpapi.RoleAdmin, http.StatusOK), c.put)
		huma.Register(api, httpapi.Operation("delete-setting", http.MethodDelete, settingPath, httpapi.RoleAdmin, http.StatusNoContent), c.delete)
		huma.Register(api, httpapi.Operation("setting-history", http.MethodGet, historyPath, httpapi.RoleWriter, http.StatusOK), c.history)
	}
}

func (c *controller) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	all, err := c.svc.List(ctx)
	if err != nil {
		return nil, httpapi.ToHumaError(ctx, err)
	}
	out := &listOutput{}
	out.Body.Settings = all
	if out.Body.Settings == nil {
		out.Body.Settings = []settings.Setting{}
	}
	return out, nil
}

func (c *controller) get(ctx context.Context, in *keyInput) (*settingOutput, error) {
	s, err := c.svc.Get(ctx, in.Key)
	if err != nil {
		return nil, httpapi.ToHumaError(ctx, err)
	}
	return &settingOutput{Body: s}, nil
}

func (c *controller) put(ctx context.Context, in *putInput) (*settingOutput, error) {
	s, err := c.svc.Put(ctx, in.Key, in.Body.Value, in.Body.Actor)
	if err != nil {
		return nil, httpapi.ToHumaError(ctx, err)
	}
	return &settingOutput{Body: s}, nil
}

func (c *controller) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	if err := c.svc.Delete(ctx, in.Key, in.Actor); err != nil {
		return nil, httpapi.ToHumaError(ctx, err)
	}
	return &emptyOutput{}, nil
}

func (c *controller) history(ctx context.Context, in *historyInput) (*historyOutput, error) {
	versions, err := c.svc.History(ctx, in.Key, in.Limit)
	if err != nil {
		return nil, httpapi.ToHumaError(ctx, err)
	}
	out := &historyOutput{}
	out.Body.Versions = versions
	if out.Body.Versions == nil {
		out.Body.Versions = []settings.Version{}
	}
	return out, nil
}
