package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

const (
	bearerPrefix = "Bearer "
	roleKey      = "role"
	roleNone     = "none"
	roleWriter   = "writer"
	roleAdmin    = "admin"
)

func tokenMatches(presented, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}

func decide(presented, requiredRole, writerToken, adminToken string) int {
	isAdmin := tokenMatches(presented, adminToken)
	isWriter := tokenMatches(presented, writerToken)

	switch {
	case !isAdmin && !isWriter:
		return http.StatusUnauthorized
	case requiredRole == roleWriter, requiredRole == roleAdmin && isAdmin:
		return http.StatusOK
	default:
		return http.StatusForbidden
	}
}

func authMiddleware(baseCtx context.Context, api huma.API, writerToken, adminToken string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		role := ""
		if op := ctx.Operation(); op != nil {
			if value, ok := op.Metadata[roleKey].(string); ok {
				role = value
			}
		}

		if role == roleNone {
			next(ctx)
			return
		}

		header := ctx.Header("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			writeDenied(baseCtx, api, ctx, http.StatusUnauthorized, "unauthorized")
			return
		}

		switch decide(header[len(bearerPrefix):], role, writerToken, adminToken) {
		case http.StatusOK:
			next(ctx)
		case http.StatusUnauthorized:
			writeDenied(baseCtx, api, ctx, http.StatusUnauthorized, "unauthorized")
		default:
			writeDenied(baseCtx, api, ctx, http.StatusForbidden, "forbidden")
		}
	}
}

func writeDenied(reqCtx context.Context, api huma.API, ctx huma.Context, status int, message string) {
	if err := huma.WriteErr(api, ctx, status, message); err != nil {
		slog.WarnContext(reqCtx, "failed to write error response", slog.String("error", err.Error()))
	}
}
