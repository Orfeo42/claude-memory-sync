package synthapi

import (
	"context"
	"net/http"

	"claude-memory-sync/internal/synthsettings"

	"github.com/danielgtaylor/huma/v2"
)

const promptPath = "/api/prompts/{name}"

type promptNameInput struct {
	Name string `path:"name" minLength:"1" maxLength:"100"`
}

type putPromptInput struct {
	Name string `path:"name" minLength:"1" maxLength:"100"`
	Body struct {
		Content string `json:"content" minLength:"1" maxLength:"200000"`
	}
}

type promptHistoryInput struct {
	Name  string `path:"name" minLength:"1" maxLength:"100"`
	Limit int    `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type promptsOutput struct {
	Body struct {
		Prompts []synthsettings.PromptInfo `json:"prompts"`
	}
}

type promptOutput struct {
	Body synthsettings.Prompt
}

type promptHistoryOutput struct {
	Body struct {
		Versions []synthsettings.PromptVersion `json:"versions"`
	}
}

func (s *server) registerPrompts(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-prompts", Method: http.MethodGet, Path: "/api/prompts"}, s.listPrompts)
	huma.Register(api, huma.Operation{OperationID: "get-prompt", Method: http.MethodGet, Path: promptPath}, s.getPrompt)
	huma.Register(api, huma.Operation{OperationID: "put-prompt", Method: http.MethodPut, Path: promptPath}, s.putPrompt)
	huma.Register(api, huma.Operation{OperationID: "reset-prompt", Method: http.MethodDelete, Path: promptPath}, s.resetPrompt)
	huma.Register(api, huma.Operation{OperationID: "prompt-history", Method: http.MethodGet, Path: "/api/prompts/{name}/history"}, s.promptHistory)
}

func (s *server) listPrompts(ctx context.Context, _ *emptyInput) (*promptsOutput, error) {
	prompts, err := s.Settings.Prompts(ctx)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if prompts == nil {
		prompts = []synthsettings.PromptInfo{}
	}

	out := &promptsOutput{}
	out.Body.Prompts = prompts
	return out, nil
}

func (s *server) getPrompt(ctx context.Context, in *promptNameInput) (*promptOutput, error) {
	prompt, err := s.Settings.GetPrompt(ctx, in.Name)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &promptOutput{Body: prompt}, nil
}

func (s *server) putPrompt(ctx context.Context, in *putPromptInput) (*promptOutput, error) {
	prompt, err := s.Settings.SetPrompt(ctx, in.Name, in.Body.Content)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &promptOutput{Body: prompt}, nil
}

func (s *server) resetPrompt(ctx context.Context, in *promptNameInput) (*promptOutput, error) {
	prompt, err := s.Settings.ResetPrompt(ctx, in.Name)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	return &promptOutput{Body: prompt}, nil
}

func (s *server) promptHistory(ctx context.Context, in *promptHistoryInput) (*promptHistoryOutput, error) {
	versions, err := s.Settings.PromptHistory(ctx, in.Name, in.Limit)
	if err != nil {
		return nil, toHumaError(ctx, err)
	}
	if versions == nil {
		versions = []synthsettings.PromptVersion{}
	}

	out := &promptHistoryOutput{}
	out.Body.Versions = versions
	return out, nil
}
