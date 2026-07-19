package api

import (
	"context"
	"net/http"

	"claude-memory-sync/internal/manifest"
	"claude-memory-sync/internal/store"

	"github.com/danielgtaylor/huma/v2"
)

const (
	clientsNamespacePrefix   = "clients/"
	canonicalNamespace       = "canonical"
	octetStreamContentType   = "application/octet-stream"
	unlimitedBodyBytes       = -1
	unlimitedBodyReadTimeout = -1
	clientFilePath           = "/v1/clients/{id}/file/{path...}"
)

type emptyInput struct{}

type emptyOutput struct{}

type treeOutput struct {
	Body manifest.Manifest
}

type fileOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

type clientIDInput struct {
	ID string `path:"id"`
}

type clientFileInput struct {
	ID   string `path:"id"`
	Path string `path:"path"`
}

type clientFilePutInput struct {
	ID      string `path:"id"`
	Path    string `path:"path"`
	RawBody []byte
}

type canonicalFileInput struct {
	Path string `path:"path"`
}

func registerOperations(api huma.API, s store.Store) {
	huma.Register(api, huma.Operation{
		OperationID:   "healthz",
		Method:        http.MethodGet,
		Path:          healthzPath,
		DefaultStatus: http.StatusOK,
	}, handleHealthz)

	huma.Register(api, huma.Operation{
		OperationID: "get-client-tree",
		Method:      http.MethodGet,
		Path:        "/v1/clients/{id}/tree",
	}, handleClientTree(s))

	huma.Register(api, huma.Operation{
		OperationID: "get-client-file",
		Method:      http.MethodGet,
		Path:        clientFilePath,
	}, handleClientFileGet(s))

	huma.Register(api, huma.Operation{
		OperationID:     "put-client-file",
		Method:          http.MethodPut,
		Path:            clientFilePath,
		MaxBodyBytes:    unlimitedBodyBytes,
		BodyReadTimeout: unlimitedBodyReadTimeout,
	}, handleClientFilePut(s))

	huma.Register(api, huma.Operation{
		OperationID: "delete-client-file",
		Method:      http.MethodDelete,
		Path:        clientFilePath,
	}, handleClientFileDelete(s))

	huma.Register(api, huma.Operation{
		OperationID: "get-canonical-tree",
		Method:      http.MethodGet,
		Path:        "/v1/canonical/tree",
	}, handleCanonicalTree(s))

	huma.Register(api, huma.Operation{
		OperationID: "get-canonical-file",
		Method:      http.MethodGet,
		Path:        "/v1/canonical/file/{path...}",
	}, handleCanonicalFileGet(s))
}

func handleHealthz(_ context.Context, _ *emptyInput) (*emptyOutput, error) {
	return &emptyOutput{}, nil
}

func handleClientTree(s store.Store) func(context.Context, *clientIDInput) (*treeOutput, error) {
	return func(ctx context.Context, input *clientIDInput) (*treeOutput, error) {
		if !validClientID(input.ID) {
			return nil, huma.Error400BadRequest("invalid client id")
		}

		tree, err := s.Tree(ctx, clientsNamespacePrefix+input.ID)
		if err != nil {
			return nil, mapStoreError(err)
		}

		return &treeOutput{Body: tree}, nil
	}
}

func handleClientFileGet(s store.Store) func(context.Context, *clientFileInput) (*fileOutput, error) {
	return func(ctx context.Context, input *clientFileInput) (*fileOutput, error) {
		if !validClientID(input.ID) {
			return nil, huma.Error400BadRequest("invalid client id")
		}

		content, err := s.Read(ctx, clientsNamespacePrefix+input.ID, input.Path)
		if err != nil {
			return nil, mapStoreError(err)
		}

		return &fileOutput{ContentType: octetStreamContentType, Body: content}, nil
	}
}

func handleClientFilePut(s store.Store) func(context.Context, *clientFilePutInput) (*emptyOutput, error) {
	return func(ctx context.Context, input *clientFilePutInput) (*emptyOutput, error) {
		if !validClientID(input.ID) {
			return nil, huma.Error400BadRequest("invalid client id")
		}

		if err := s.Write(ctx, clientsNamespacePrefix+input.ID, input.Path, input.RawBody, input.ID); err != nil {
			return nil, mapStoreError(err)
		}

		return &emptyOutput{}, nil
	}
}

func handleClientFileDelete(s store.Store) func(context.Context, *clientFileInput) (*emptyOutput, error) {
	return func(ctx context.Context, input *clientFileInput) (*emptyOutput, error) {
		if !validClientID(input.ID) {
			return nil, huma.Error400BadRequest("invalid client id")
		}

		if err := s.Delete(ctx, clientsNamespacePrefix+input.ID, input.Path, input.ID); err != nil {
			return nil, mapStoreError(err)
		}

		return &emptyOutput{}, nil
	}
}

func handleCanonicalTree(s store.Store) func(context.Context, *emptyInput) (*treeOutput, error) {
	return func(ctx context.Context, _ *emptyInput) (*treeOutput, error) {
		tree, err := s.Tree(ctx, canonicalNamespace)
		if err != nil {
			return nil, mapStoreError(err)
		}

		return &treeOutput{Body: tree}, nil
	}
}

func handleCanonicalFileGet(s store.Store) func(context.Context, *canonicalFileInput) (*fileOutput, error) {
	return func(ctx context.Context, input *canonicalFileInput) (*fileOutput, error) {
		content, err := s.Read(ctx, canonicalNamespace, input.Path)
		if err != nil {
			return nil, mapStoreError(err)
		}

		return &fileOutput{ContentType: octetStreamContentType, Body: content}, nil
	}
}
