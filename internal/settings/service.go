package settings

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context) ([]Setting, error) {
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if Key(all[i].Key).IsSecret() {
			all[i].Value = nil
		}
	}
	return all, nil
}

func (s *Service) Get(ctx context.Context, rawKey string) (Setting, error) {
	key, err := ParseKey(rawKey)
	if err != nil {
		return Setting{}, err
	}
	return s.repo.Get(ctx, key)
}

func (s *Service) Put(ctx context.Context, rawKey, value, actor string) (Setting, error) {
	key, err := ParseKey(rawKey)
	if err != nil {
		return Setting{}, err
	}
	saved, err := s.repo.Put(ctx, PutRequest{Key: key, Value: value, Actor: actor})
	if err != nil {
		return Setting{}, err
	}
	if key.IsSecret() {
		saved.Value = nil
	}
	return saved, nil
}

func (s *Service) Delete(ctx context.Context, rawKey, actor string) error {
	key, err := ParseKey(rawKey)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, DeleteRequest{Key: key, Actor: actor})
}

func (s *Service) History(ctx context.Context, rawKey string, limit int) ([]Version, error) {
	key, err := ParseKey(rawKey)
	if err != nil {
		return nil, err
	}
	return s.repo.History(ctx, key, limit)
}
