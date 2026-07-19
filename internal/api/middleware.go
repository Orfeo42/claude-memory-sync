package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

const (
	healthzPath  = "/v1/healthz"
	bearerPrefix = "Bearer "
)

func authMiddleware(api huma.API, token string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.URL().Path == healthzPath {
			next(ctx)
			return
		}

		header := ctx.Header("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) || header[len(bearerPrefix):] != token {
			if err := huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized"); err != nil {
				slog.WarnContext(ctx.Context(), "failed to write error response", slog.String("error", err.Error()))
			}
			return
		}

		next(ctx)
	}
}
