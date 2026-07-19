package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/Orfeo42/claude-memory-sync/internal/store"
)

const apiTitle = "memory-sync"
const apiVersion = "1.0.0"

func New(s store.Store, token string) http.Handler {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig(apiTitle, apiVersion))

	humaAPI.UseMiddleware(authMiddleware(humaAPI, token))
	registerOperations(humaAPI, s)

	return mux
}
