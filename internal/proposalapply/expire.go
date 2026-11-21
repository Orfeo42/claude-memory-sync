package proposalapply

import (
	"context"
	"time"
)

type ExpireResult struct {
	Expired int `json:"expired"`
}

func Expire(ctx context.Context, p Pipeline, actor string, olderThan time.Duration) (ExpireResult, error) {
	expired, err := p.Expire(ctx, actor, int(olderThan.Hours()))
	if err != nil {
		return ExpireResult{}, err
	}
	return ExpireResult{Expired: expired}, nil
}
