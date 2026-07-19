package api

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

const healthzPath = "/v1/healthz"
const bearerPrefix = "Bearer "

func authMiddleware(api huma.API, token string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.URL().Path == healthzPath {
			next(ctx)
			return
		}

		header := ctx.Header("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) || header[len(bearerPrefix):] != token {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized")
			return
		}

		next(ctx)
	}
}
