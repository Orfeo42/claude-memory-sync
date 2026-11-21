package settings

import "context"

//go:generate go tool moq -out repository_mock.go . Repository:MockRepository
type Repository interface {
	List(ctx context.Context) ([]Setting, error)
	Get(ctx context.Context, key Key) (Setting, error)
	Put(ctx context.Context, req PutRequest) (Setting, error)
	Delete(ctx context.Context, req DeleteRequest) error
	History(ctx context.Context, key Key, limit int) ([]Version, error)
}
