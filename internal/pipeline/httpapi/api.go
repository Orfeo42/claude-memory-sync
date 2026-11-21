package httpapi

import (
	"context"
	"errors"
	"net/http"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const (
	apiTitle   = "pipeline"
	apiVersion = "1.0.0"
)

type Registrar func(api huma.API)

func New(ctx context.Context, svc *pipeline.Service, writerToken, adminToken string, registrars ...Registrar) http.Handler {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig(apiTitle, apiVersion))

	humaAPI.UseMiddleware(authMiddleware(ctx, humaAPI, writerToken, adminToken))
	registerOperations(humaAPI, &controller{svc: svc})
	for _, register := range registrars {
		register(humaAPI)
	}

	return mux
}

func ToHumaError(ctx context.Context, err error) error {
	status := domain.StatusCodeFromError(err)

	switch {
	case errors.Is(err, domain.ErrUpstream):
		domain.LogWarn(ctx, "pipeline request failed upstream", err)
	case status >= http.StatusInternalServerError:
		domain.LogError(ctx, "pipeline request failed", err)
	}

	return huma.NewError(status, domain.UserMessage(err))
}
