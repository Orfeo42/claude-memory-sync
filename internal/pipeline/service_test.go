package pipeline

import (
	"context"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	secretContent = "api_key = abcdefghijklmnopqrstuvwxyz123456"
	rulePath      = "global/rules/go.md"
)

func strPtr(v string) *string {
	return &v
}

func TestNewService(t *testing.T) {
	t.Run("holds the repository", func(t *testing.T) {
		repo := &MockRepository{}
		assert.Same(t, repo, NewService(repo).repo)
	})
}

func TestServiceCreate(t *testing.T) {
	t.Run("creates a rule proposal with derived kind and name", func(t *testing.T) {
		repo := &MockRepository{
			CreateFunc: func(_ context.Context, p NewProposal) (Proposal, error) {
				return Proposal{ID: "p1", Kind: p.Kind, Name: p.Name, Status: StatusPending, Files: p.Files}, nil
			},
		}
		files := []File{{TargetPath: rulePath, Content: "x"}}

		got, err := NewService(repo).Create(context.Background(), "flow", "run", files)

		require.NoError(t, err)
		assert.Equal(t, KindRule, got.Kind)
		assert.Equal(t, "go.md", got.Name)
		require.Len(t, repo.CreateCalls(), 1)
		assert.Equal(t, "flow", repo.CreateCalls()[0].P.SourceFlow)
		assert.Equal(t, "run", repo.CreateCalls()[0].P.SourceRun)
	})

	t.Run("accepts several files of one skill", func(t *testing.T) {
		repo := &MockRepository{
			CreateFunc: func(_ context.Context, p NewProposal) (Proposal, error) {
				return Proposal{ID: "p1", Kind: p.Kind, Name: p.Name}, nil
			},
		}
		files := []File{
			{TargetPath: "global/skills/foo/SKILL.md", Content: "a"},
			{TargetPath: "global/skills/foo/run.sh", Content: "b"},
		}

		got, err := NewService(repo).Create(context.Background(), "f", "r", files)

		require.NoError(t, err)
		assert.Equal(t, KindSkill, got.Kind)
		assert.Equal(t, "foo", got.Name)
	})

	t.Run("rejects empty files", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Create(context.Background(), "f", "r", nil)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.CreateCalls())
	})

	t.Run("rejects invalid target", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Create(context.Background(), "f", "r", []File{{TargetPath: "other/x.md", Content: "x"}})

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.CreateCalls())
	})

	t.Run("rejects duplicate target", func(t *testing.T) {
		repo := &MockRepository{}
		files := []File{{TargetPath: rulePath, Content: "a"}, {TargetPath: rulePath, Content: "b"}}

		_, err := NewService(repo).Create(context.Background(), "f", "r", files)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.CreateCalls())
	})

	t.Run("rejects secret content", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Create(context.Background(), "f", "r", []File{{TargetPath: rulePath, Content: secretContent}})

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.CreateCalls())
	})

	t.Run("rejects files from different groups", func(t *testing.T) {
		repo := &MockRepository{}
		files := []File{{TargetPath: rulePath, Content: "a"}, {TargetPath: "global/rules/py.md", Content: "b"}}

		_, err := NewService(repo).Create(context.Background(), "f", "r", files)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.CreateCalls())
	})

	t.Run("propagates repository conflict", func(t *testing.T) {
		repo := &MockRepository{
			CreateFunc: func(_ context.Context, _ NewProposal) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrConflict, "open proposal")
			},
		}

		_, err := NewService(repo).Create(context.Background(), "f", "r", []File{{TargetPath: rulePath, Content: "x"}})

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceGet(t *testing.T) {
	t.Run("returns proposal with events using two repository calls", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc:    func(_ context.Context, id string) (Proposal, error) { return Proposal{ID: id}, nil },
			EventsFunc: func(_ context.Context, _ string) ([]Event, error) { return []Event{{ID: 1}, {ID: 2}}, nil },
		}

		got, err := NewService(repo).Get(context.Background(), "p1")

		require.NoError(t, err)
		assert.Equal(t, "p1", got.Proposal.ID)
		assert.Len(t, got.Events, 2)
		assert.Len(t, repo.GetCalls(), 1)
		assert.Len(t, repo.EventsCalls(), 1)
	})

	t.Run("propagates not found without reading events", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc: func(_ context.Context, _ string) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrNotFound, "missing")
			},
		}

		_, err := NewService(repo).Get(context.Background(), "p1")

		require.ErrorIs(t, err, domain.ErrNotFound)
		assert.Empty(t, repo.EventsCalls())
	})

	t.Run("propagates events error", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc:    func(_ context.Context, _ string) (Proposal, error) { return Proposal{}, nil },
			EventsFunc: func(_ context.Context, _ string) ([]Event, error) { return nil, domain.ErrInternal },
		}

		_, err := NewService(repo).Get(context.Background(), "p1")

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceList(t *testing.T) {
	t.Run("defaults limit to 100", func(t *testing.T) {
		repo := &MockRepository{
			ListFunc: func(_ context.Context, _ ListFilter) ([]Proposal, error) { return []Proposal{{ID: "a"}}, nil },
		}

		got, err := NewService(repo).List(context.Background(), ListFilter{})

		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, 100, repo.ListCalls()[0].F.Limit)
	})

	t.Run("keeps explicit limit", func(t *testing.T) {
		repo := &MockRepository{
			ListFunc: func(_ context.Context, _ ListFilter) ([]Proposal, error) { return nil, nil },
		}

		_, err := NewService(repo).List(context.Background(), ListFilter{Limit: 7})

		require.NoError(t, err)
		assert.Equal(t, 7, repo.ListCalls()[0].F.Limit)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{
			ListFunc: func(_ context.Context, _ ListFilter) ([]Proposal, error) { return nil, domain.ErrInternal },
		}

		_, err := NewService(repo).List(context.Background(), ListFilter{})

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func transitionRepo() *MockRepository {
	return &MockRepository{
		TransitionFunc: func(_ context.Context, tr Transition) (Proposal, error) {
			return Proposal{ID: tr.ID, Status: tr.To}, nil
		},
	}
}

func TestServiceDecide(t *testing.T) {
	t.Run("approve moves pending to approved", func(t *testing.T) {
		repo := transitionRepo()

		got, err := NewService(repo).Decide(context.Background(), "p1", "alice", true, nil)

		require.NoError(t, err)
		assert.Equal(t, StatusApproved, got.Status)
		call := repo.TransitionCalls()[0].T
		assert.Equal(t, StatusPending, call.From)
		assert.Equal(t, "alice", call.Actor)
		assert.Nil(t, call.Note)
	})

	t.Run("reject moves pending to rejected with reason note", func(t *testing.T) {
		repo := transitionRepo()

		got, err := NewService(repo).Decide(context.Background(), "p1", "alice", false, strPtr("nope"))

		require.NoError(t, err)
		assert.Equal(t, StatusRejected, got.Status)
		assert.Equal(t, "nope", *repo.TransitionCalls()[0].T.Note)
	})

	t.Run("propagates conflict", func(t *testing.T) {
		repo := &MockRepository{
			TransitionFunc: func(_ context.Context, _ Transition) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrConflict, "not pending")
			},
		}

		_, err := NewService(repo).Decide(context.Background(), "p1", "a", true, nil)

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceRevise(t *testing.T) {
	t.Run("replaces files when group matches", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc: func(_ context.Context, id string) (Proposal, error) { return Proposal{ID: id, Name: "go.md"}, nil },
			ReplaceFilesFunc: func(_ context.Context, id, _ string, files []File) (Proposal, error) {
				return Proposal{ID: id, Files: files}, nil
			},
		}

		got, err := NewService(repo).Revise(context.Background(), "p1", "alice", []File{{TargetPath: rulePath, Content: "new"}})

		require.NoError(t, err)
		assert.Equal(t, "new", got.Files[0].Content)
		assert.Equal(t, "alice", repo.ReplaceFilesCalls()[0].Actor)
	})

	t.Run("rejects group change", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc: func(_ context.Context, id string) (Proposal, error) { return Proposal{ID: id, Name: "other.md"}, nil },
		}

		_, err := NewService(repo).Revise(context.Background(), "p1", "alice", []File{{TargetPath: rulePath, Content: "new"}})

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.ReplaceFilesCalls())
	})

	t.Run("rejects invalid files before reading", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Revise(context.Background(), "p1", "alice", []File{{TargetPath: rulePath, Content: secretContent}})

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.GetCalls())
	})

	t.Run("propagates not found", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc: func(_ context.Context, _ string) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrNotFound, "missing")
			},
		}

		_, err := NewService(repo).Revise(context.Background(), "p1", "alice", []File{{TargetPath: rulePath, Content: "x"}})

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("propagates replace conflict", func(t *testing.T) {
		repo := &MockRepository{
			GetFunc: func(_ context.Context, _ string) (Proposal, error) { return Proposal{Name: "go.md"}, nil },
			ReplaceFilesFunc: func(_ context.Context, _, _ string, _ []File) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrConflict, "not pending")
			},
		}

		_, err := NewService(repo).Revise(context.Background(), "p1", "alice", []File{{TargetPath: rulePath, Content: "x"}})

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceRetry(t *testing.T) {
	t.Run("moves failed to approved", func(t *testing.T) {
		repo := transitionRepo()

		got, err := NewService(repo).Retry(context.Background(), "p1", "alice")

		require.NoError(t, err)
		assert.Equal(t, StatusApproved, got.Status)
		assert.Equal(t, StatusFailed, repo.TransitionCalls()[0].T.From)
	})

	t.Run("propagates conflict", func(t *testing.T) {
		repo := &MockRepository{
			TransitionFunc: func(_ context.Context, _ Transition) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrConflict, "not failed")
			},
		}

		_, err := NewService(repo).Retry(context.Background(), "p1", "alice")

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceMarkApplied(t *testing.T) {
	t.Run("moves applying to applied with sha", func(t *testing.T) {
		repo := transitionRepo()

		got, err := NewService(repo).MarkApplied(context.Background(), "p1", "worker", "abc123")

		require.NoError(t, err)
		assert.Equal(t, StatusApplied, got.Status)
		call := repo.TransitionCalls()[0].T
		assert.Equal(t, StatusApplying, call.From)
		assert.Equal(t, "abc123", *call.AppliedSHA)
	})

	t.Run("propagates not found", func(t *testing.T) {
		repo := &MockRepository{
			TransitionFunc: func(_ context.Context, _ Transition) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrNotFound, "missing")
			},
		}

		_, err := NewService(repo).MarkApplied(context.Background(), "p1", "w", "sha")

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestServiceMarkFailed(t *testing.T) {
	t.Run("moves applying to failed with note", func(t *testing.T) {
		repo := transitionRepo()

		got, err := NewService(repo).MarkFailed(context.Background(), "p1", "worker", "boom")

		require.NoError(t, err)
		assert.Equal(t, StatusFailed, got.Status)
		call := repo.TransitionCalls()[0].T
		assert.Equal(t, StatusApplying, call.From)
		assert.Equal(t, "boom", *call.Note)
	})

	t.Run("propagates conflict", func(t *testing.T) {
		repo := &MockRepository{
			TransitionFunc: func(_ context.Context, _ Transition) (Proposal, error) {
				return Proposal{}, domain.Error(domain.ErrConflict, "not applying")
			},
		}

		_, err := NewService(repo).MarkFailed(context.Background(), "p1", "w", "boom")

		require.ErrorIs(t, err, domain.ErrConflict)
	})
}

func TestServiceTransition(t *testing.T) {
	t.Run("refuses an edge the status machine forbids", func(t *testing.T) {
		repo := transitionRepo()

		_, err := NewService(repo).transition(context.Background(), StatusApplied, StatusPending, Transition{ID: "p1"})

		require.ErrorIs(t, err, domain.ErrConflict)
		assert.Empty(t, repo.TransitionCalls())
	})
}

func TestServiceClaim(t *testing.T) {
	t.Run("defaults lease to 10 minutes", func(t *testing.T) {
		repo := &MockRepository{
			ClaimFunc: func(_ context.Context, _ string, _ time.Duration) (Proposal, error) {
				return Proposal{ID: "p1"}, nil
			},
		}

		got, err := NewService(repo).Claim(context.Background(), "w", 0)

		require.NoError(t, err)
		assert.Equal(t, "p1", got.ID)
		assert.Equal(t, 10*time.Minute, repo.ClaimCalls()[0].Lease)
	})

	t.Run("returns ErrNothingToClaim when nothing is claimable", func(t *testing.T) {
		repo := &MockRepository{
			ClaimFunc: func(_ context.Context, _ string, _ time.Duration) (Proposal, error) {
				return Proposal{}, ErrNothingToClaim
			},
		}

		_, err := NewService(repo).Claim(context.Background(), "w", time.Minute)

		require.ErrorIs(t, err, ErrNothingToClaim)
	})

	t.Run("rejects lease over one hour", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Claim(context.Background(), "w", time.Hour+time.Second)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.ClaimCalls())
	})

	t.Run("rejects negative lease", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Claim(context.Background(), "w", -time.Second)

		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{
			ClaimFunc: func(_ context.Context, _ string, _ time.Duration) (Proposal, error) {
				return Proposal{}, domain.ErrInternal
			},
		}

		_, err := NewService(repo).Claim(context.Background(), "w", time.Minute)

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceExpire(t *testing.T) {
	t.Run("returns expired count", func(t *testing.T) {
		repo := &MockRepository{
			ExpirePendingFunc: func(_ context.Context, _ time.Duration, _ string) (int, error) { return 3, nil },
		}

		n, err := NewService(repo).Expire(context.Background(), 24*time.Hour, "cron")

		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, "cron", repo.ExpirePendingCalls()[0].Actor)
	})

	t.Run("rejects non positive duration", func(t *testing.T) {
		repo := &MockRepository{}

		_, err := NewService(repo).Expire(context.Background(), 0, "cron")

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.ExpirePendingCalls())
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{
			ExpirePendingFunc: func(_ context.Context, _ time.Duration, _ string) (int, error) { return 0, domain.ErrInternal },
		}

		_, err := NewService(repo).Expire(context.Background(), time.Hour, "cron")

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceRejections(t *testing.T) {
	t.Run("passes kind filter through", func(t *testing.T) {
		kind := KindRule
		repo := &MockRepository{
			RejectionsFunc: func(_ context.Context, _ *Kind) ([]Rejection, error) { return []Rejection{{Name: "x"}}, nil },
		}

		got, err := NewService(repo).Rejections(context.Background(), &kind)

		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, &kind, repo.RejectionsCalls()[0].Kind)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{
			RejectionsFunc: func(_ context.Context, _ *Kind) ([]Rejection, error) { return nil, domain.ErrInternal },
		}

		_, err := NewService(repo).Rejections(context.Background(), nil)

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}

func TestServiceAddRejection(t *testing.T) {
	t.Run("upserts the rejection", func(t *testing.T) {
		reason := "noisy"
		repo := &MockRepository{
			UpsertRejectionFunc: func(_ context.Context, _ Rejection) error { return nil },
		}

		err := NewService(repo).AddRejection(context.Background(), KindRule, "go", "alice", &reason)

		require.NoError(t, err)
		calls := repo.UpsertRejectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, Rejection{Kind: KindRule, Name: "go", Reason: &reason}, calls[0].R)
	})

	t.Run("unknown kind is a validation error", func(t *testing.T) {
		repo := &MockRepository{}

		err := NewService(repo).AddRejection(context.Background(), Kind("bogus"), "go", "alice", nil)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.UpsertRejectionCalls())
	})

	t.Run("empty name is a validation error", func(t *testing.T) {
		repo := &MockRepository{}

		err := NewService(repo).AddRejection(context.Background(), KindRule, "", "alice", nil)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.UpsertRejectionCalls())
	})

	t.Run("empty actor is a validation error", func(t *testing.T) {
		repo := &MockRepository{}

		err := NewService(repo).AddRejection(context.Background(), KindRule, "go", "", nil)

		require.ErrorIs(t, err, domain.ErrValidation)
		assert.Empty(t, repo.UpsertRejectionCalls())
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := &MockRepository{
			UpsertRejectionFunc: func(_ context.Context, _ Rejection) error { return domain.ErrInternal },
		}

		err := NewService(repo).AddRejection(context.Background(), KindRule, "go", "alice", nil)

		require.ErrorIs(t, err, domain.ErrInternal)
	})
}
