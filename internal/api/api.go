package api

import (
	"net/http"

	"claude-memory-sync/internal/store"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const (
	apiTitle   = "memory-sync"
	apiVersion = "1.0.0"
)

func New(s store.Store, token string) http.Handler {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig(apiTitle, apiVersion))

	humaAPI.UseMiddleware(authMiddleware(humaAPI, token))
	registerOperations(humaAPI, s)

	return mux
}
