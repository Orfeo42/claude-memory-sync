package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const (
	apiTitle   = "memory-sync"
	apiVersion = "1.0.0"
)

func New(token string, gitHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig(apiTitle, apiVersion))

	humaAPI.UseMiddleware(authMiddleware(humaAPI, token))
	registerOperations(humaAPI)
	mux.Handle("/git/", requireBearerToken(token, gitHandler))

	return mux
}
