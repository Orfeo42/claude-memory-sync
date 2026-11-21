package synthapi

import (
	"context"
	"net/http"

	"claude-memory-sync/internal/synthsettings"

	"github.com/danielgtaylor/huma/v2"
)

const (
	paramPath = "/api/settings/params/{key}"
	tokenPath = "/api/settings/token"
)

type paramKeyInput struct {
	Key string `path:"key" minLength:"1" maxLength:"100"`
}

type putParamInput struct {
	Key  string `path:"key" minLength:"1" maxLength:"100"`
	Body struct {
		Value string `json:"value" maxLength:"1000"`
	}
}

type putTokenInput struct {
	Body struct {
		Token string `json:"token" minLength:"1"`
	}
}

type settingsOutput struct {
	Body struct {
		Params []synthsettings.Param     `json:"params"`
		Token  synthsettings.TokenStatus `json:"token"`
	}
}

type paramOutput struct {
	Body synthsettings.Param
}

type tokenOutput struct {
	Body synthsettings.TokenStatus
}

func (s *server) registerSettings(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "get-settings", Method: http.MethodGet, Path: "/api/settings"}, s.getSettings)
	huma.Register(api, huma.Operation{OperationID: "put-param", Method: http.MethodPut, Path: paramPath}, s.putParam)
	huma.Register(api, huma.Operation{OperationID: "reset-param", Method: http.MethodDelete, Path: paramPath}, s.resetParam)
	huma.Register(api, huma.Operation{OperationID: "put-token", Method: http.MethodPut, Path: tokenPath}, s.putToken)
	huma.Register(api, huma.Operation{OperationID: "clear-token", Method: http.MethodDelete, Path: tokenPath}, s.clearToken)
}

func (s *server) getSettings(ctx context.Context, _ *emptyInput) (*settingsOutput, error) {
	params, err := s.Settings.Params(ctx)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	token, err := s.Settings.TokenStatus(ctx)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if params == nil {
		params = []synthsettings.Param{}
	}

	out := &settingsOutput{}
	out.Body.Params = params
	out.Body.Token = token
	return out, nil
}

func (s *server) putParam(ctx context.Context, in *putParamInput) (*paramOutput, error) {
	param, err := s.Settings.SetParam(ctx, in.Key, in.Body.Value)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &paramOutput{Body: param}, nil
}

func (s *server) resetParam(ctx context.Context, in *paramKeyInput) (*paramOutput, error) {
	param, err := s.Settings.ResetParam(ctx, in.Key)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &paramOutput{Body: param}, nil
}

func (s *server) putToken(ctx context.Context, in *putTokenInput) (*tokenOutput, error) {
	status, err := s.Settings.SetToken(ctx, in.Body.Token)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &tokenOutput{Body: status}, nil
}

func (s *server) clearToken(ctx context.Context, _ *emptyInput) (*tokenOutput, error) {
	status, err := s.Settings.ClearToken(ctx)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &tokenOutput{Body: status}, nil
}
