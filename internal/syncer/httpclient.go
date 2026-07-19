package syncer

//go:generate moq -out httpclient_mock.go . HTTPClient:MockHTTPClient

import (
	"context"

	"github.com/Orfeo42/claude-memory-sync/internal/manifest"
)

type HTTPClient interface {
	ClientTree(ctx context.Context) (manifest.Manifest, error)
	PutClientFile(ctx context.Context, path string, content []byte) error
	DeleteClientFile(ctx context.Context, path string) error
	CanonicalTree(ctx context.Context) (manifest.Manifest, error)
	GetCanonicalFile(ctx context.Context, path string) ([]byte, error)
}
