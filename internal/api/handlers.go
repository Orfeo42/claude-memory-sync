package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type emptyInput struct{}

type emptyOutput struct{}

func registerOperations(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "healthz",
		Method:        http.MethodGet,
		Path:          healthzPath,
		DefaultStatus: http.StatusOK,
	}, handleHealthz)
}

func handleHealthz(_ context.Context, _ *emptyInput) (*emptyOutput, error) {
	return &emptyOutput{}, nil
}
