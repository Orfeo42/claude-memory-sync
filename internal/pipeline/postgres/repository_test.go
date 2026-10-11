package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
)

func TestCreate(t *testing.T) {
	ctx := t.Context()

	t.Run("persists proposal files and pending event", func(t *testing.T) {
		repo := setup(ctx, t)
		p := skillProposal("alpha")
		p.Files = append(p.Files, pipeline.File{TargetPath: "global/skills/alpha/extra.md", Content: "extra"})

		created, err := repo.Create(ctx, p)
		require.NoError(t, err)

		assert.NotEmpty(t, created.ID)
		assert.Equal(t, pipeline.StatusPending, created.Status)
		assert.Equal(t, pipeline.KindSkill, created.Kind)
		assert.Equal(t, "alpha", created.Name)
		assert.Equal(t, "daily-synth", created.SourceFlow)
		assert.Equal(t, "run-1", created.SourceRun)
		assert.Equal(t, []pipeline.File{
			{TargetPath: "global/skills/alpha/SKILL.md", Content: "body of alpha"},
			{TargetPath: "global/skills/alpha/extra.md", Content: "extra"},
		}, created.Files)

		events, err := repo.Events(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Nil(t, events[0].FromStatus)
		assert.Equal(t, pipeline.StatusPending, events[0].ToStatus)
		assert.Equal(t, "daily-synth", events[0].Actor)
	})

	t.Run("rejects empty files", func(t *testing.T) {
		repo := setup(ctx, t)
		p := skillProposal("alpha")
		p.Files = nil

		_, err := repo.Create(ctx, p)
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("conflicts with open proposal on same target", func(t *testing.T) {
		repo := setup(ctx, t)
		first := mustCreate(ctx, t, repo, skillProposal("alpha"))

		_, err := repo.Create(ctx, skillProposal("alpha"))
		require.ErrorIs(t, err, domain.ErrConflict)

		mustMove(ctx, t, repo, first.ID, pipeline.StatusPending, pipeline.StatusApproved)
		_, err = repo.Create(ctx, skillProposal("alpha"))
		require.ErrorIs(t, err, domain.ErrConflict)

		mustMove(ctx, t, repo, first.ID, pipeline.StatusApproved, pipeline.StatusApplying)
		_, err = repo.Create(ctx, skillProposal("alpha"))
		require.ErrorIs(t, err, domain.ErrConflict)
	})

	t.Run("allowed when existing proposal is applied", func(t *testing.T) {
		repo := setup(ctx, t)
		first := mustApprove(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, first.ID, pipeline.StatusApproved, pipeline.StatusApplying)
		mustMove(ctx, t, repo, first.ID, pipeline.StatusApplying, pipeline.StatusApplied)

		_, err := repo.Create(ctx, skillProposal("alpha"))
		require.NoError(t, err)
	})

	t.Run("allowed when existing proposal is rejected", func(t *testing.T) {
		repo := setup(ctx, t)
		first := mustCreate(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, first.ID, pipeline.StatusPending, pipeline.StatusRejected)

		_, err := repo.Create(ctx, skillProposal("alpha"))
		require.NoError(t, err)
	})

	t.Run("allowed when existing proposal is expired", func(t *testing.T) {
		repo := setup(ctx, t)
		first := mustCreate(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, first.ID, pipeline.StatusPending, pipeline.StatusExpired)

		_, err := repo.Create(ctx, skillProposal("alpha"))
		require.NoError(t, err)
	})

	t.Run("concurrent creates on same target yield one success and one conflict", func(t *testing.T) {
		repo := setup(ctx, t)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = repo.Create(ctx, skillProposal("alpha"))
			}()
		}
		wg.Wait()

		successes := 0
		conflicts := 0
		for _, err := range errs {
			if err == nil {
				successes++
				continue
			}
			require.ErrorIs(t, err, domain.ErrConflict)
			conflicts++
		}
		assert.Equal(t, 1, successes)
		assert.Equal(t, 1, conflicts)
	})
}

func TestGet(t *testing.T) {
	ctx := t.Context()

	t.Run("returns proposal with files", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))

		got, err := repo.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.ID)
		assert.Equal(t, created.Files, got.Files)
	})

	t.Run("missing id is not found", func(t *testing.T) {
		repo := setup(ctx, t)

		_, err := repo.Get(ctx, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("malformed id is not found", func(t *testing.T) {
		repo := setup(ctx, t)

		_, err := repo.Get(ctx, "not-a-uuid")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestList(t *testing.T) {
	ctx := t.Context()

	t.Run("filters by status and kind", func(t *testing.T) {
		repo := setup(ctx, t)
		skill := mustCreate(ctx, t, repo, skillProposal("alpha"))
		rule := mustCreate(ctx, t, repo, ruleProposal("beta"))
		mustMove(ctx, t, repo, rule.ID, pipeline.StatusPending, pipeline.StatusApproved)

		approved := pipeline.StatusApproved
		got, err := repo.List(ctx, pipeline.ListFilter{Status: &approved, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, rule.ID, got[0].ID)

		kindSkill := pipeline.KindSkill
		got, err = repo.List(ctx, pipeline.ListFilter{Kind: &kindSkill, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, skill.ID, got[0].ID)
	})

	t.Run("orders newest first and includes files", func(t *testing.T) {
		repo := setup(ctx, t)
		oldest := mustCreate(ctx, t, repo, skillProposal("alpha"))
		middle := mustCreate(ctx, t, repo, skillProposal("beta"))
		newest := mustCreate(ctx, t, repo, skillProposal("gamma"))
		backdateCreated(ctx, t, oldest.ID, 3*time.Hour)
		backdateCreated(ctx, t, middle.ID, 2*time.Hour)

		got, err := repo.List(ctx, pipeline.ListFilter{Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, []string{newest.ID, middle.ID, oldest.ID}, []string{got[0].ID, got[1].ID, got[2].ID})
		for _, proposal := range got {
			assert.Len(t, proposal.Files, 1)
		}
	})

	t.Run("before filters on created_at", func(t *testing.T) {
		repo := setup(ctx, t)
		old := mustCreate(ctx, t, repo, skillProposal("alpha"))
		mustCreate(ctx, t, repo, skillProposal("beta"))
		backdateCreated(ctx, t, old.ID, 3*time.Hour)

		before := time.Now().Add(-time.Hour)
		got, err := repo.List(ctx, pipeline.ListFilter{Before: &before, Limit: 10})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, old.ID, got[0].ID)
	})

	t.Run("limit caps result", func(t *testing.T) {
		repo := setup(ctx, t)
		mustCreate(ctx, t, repo, skillProposal("alpha"))
		mustCreate(ctx, t, repo, skillProposal("beta"))
		mustCreate(ctx, t, repo, skillProposal("gamma"))

		got, err := repo.List(ctx, pipeline.ListFilter{Limit: 2})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("empty result is empty slice", func(t *testing.T) {
		repo := setup(ctx, t)

		got, err := repo.List(ctx, pipeline.ListFilter{Limit: 10})
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestEvents(t *testing.T) {
	ctx := t.Context()

	t.Run("returns history in order", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustApprove(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, created.ID, pipeline.StatusApproved, pipeline.StatusApplying)

		events, err := repo.Events(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, pipeline.StatusPending, events[0].ToStatus)
		assert.Equal(t, pipeline.StatusApproved, events[1].ToStatus)
		require.NotNil(t, events[1].FromStatus)
		assert.Equal(t, pipeline.StatusPending, *events[1].FromStatus)
		assert.Equal(t, pipeline.StatusApplying, events[2].ToStatus)
		assert.Equal(t, "tester", events[2].Actor)
	})

	t.Run("unknown proposal yields empty slice", func(t *testing.T) {
		repo := setup(ctx, t)

		events, err := repo.Events(ctx, "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.NotNil(t, events)
		assert.Empty(t, events)
	})
}

func TestTransition(t *testing.T) {
	ctx := t.Context()

	t.Run("approved sets decided fields", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))

		moved := mustMove(ctx, t, repo, created.ID, pipeline.StatusPending, pipeline.StatusApproved)
		assert.Equal(t, pipeline.StatusApproved, moved.Status)
		require.NotNil(t, moved.DecidedAt)
		require.NotNil(t, moved.DecidedBy)
		assert.Equal(t, "tester", *moved.DecidedBy)
		assert.Len(t, moved.Files, 1)
	})

	t.Run("rejected sets decided fields and upserts rejection", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))
		note := "not useful"

		moved, err := repo.Transition(ctx, pipeline.Transition{
			ID: created.ID, From: pipeline.StatusPending, To: pipeline.StatusRejected, Actor: "tester", Note: &note,
		})
		require.NoError(t, err)
		assert.Equal(t, pipeline.StatusRejected, moved.Status)
		require.NotNil(t, moved.DecidedAt)

		rejections, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, rejections, 1)
		assert.Equal(t, pipeline.KindSkill, rejections[0].Kind)
		assert.Equal(t, "alpha", rejections[0].Name)
		require.NotNil(t, rejections[0].Reason)
		assert.Equal(t, "not useful", *rejections[0].Reason)
	})

	t.Run("rejecting twice updates the same rejection row", func(t *testing.T) {
		repo := setup(ctx, t)
		first := mustCreate(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, first.ID, pipeline.StatusPending, pipeline.StatusRejected)
		second := mustCreate(ctx, t, repo, skillProposal("alpha"))
		note := "still no"
		_, err := repo.Transition(ctx, pipeline.Transition{
			ID: second.ID, From: pipeline.StatusPending, To: pipeline.StatusRejected, Actor: "tester", Note: &note,
		})
		require.NoError(t, err)

		rejections, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, rejections, 1)
		require.NotNil(t, rejections[0].Reason)
		assert.Equal(t, "still no", *rejections[0].Reason)
	})

	t.Run("applying sets claim and leaving clears it", func(t *testing.T) {
		repo := setup(ctx, t)
		approved := mustApprove(ctx, t, repo, skillProposal("alpha"))

		applying := mustMove(ctx, t, repo, approved.ID, pipeline.StatusApproved, pipeline.StatusApplying)
		require.NotNil(t, applying.ClaimedBy)
		assert.Equal(t, "tester", *applying.ClaimedBy)
		require.NotNil(t, applying.ClaimedUntil)

		sha := "abc123"
		applied, err := repo.Transition(ctx, pipeline.Transition{
			ID: approved.ID, From: pipeline.StatusApplying, To: pipeline.StatusApplied, Actor: "tester", AppliedSHA: &sha,
		})
		require.NoError(t, err)
		assert.Nil(t, applied.ClaimedBy)
		assert.Nil(t, applied.ClaimedUntil)
		require.NotNil(t, applied.AppliedSHA)
		assert.Equal(t, "abc123", *applied.AppliedSHA)
	})

	t.Run("failed sets error and approved from failed clears it", func(t *testing.T) {
		repo := setup(ctx, t)
		approved := mustApprove(ctx, t, repo, skillProposal("alpha"))
		mustMove(ctx, t, repo, approved.ID, pipeline.StatusApproved, pipeline.StatusApplying)
		note := "push rejected"

		failed, err := repo.Transition(ctx, pipeline.Transition{
			ID: approved.ID, From: pipeline.StatusApplying, To: pipeline.StatusFailed, Actor: "tester", Note: &note,
		})
		require.NoError(t, err)
		assert.Nil(t, failed.ClaimedBy)
		assert.Nil(t, failed.ClaimedUntil)
		require.NotNil(t, failed.Error)
		assert.Equal(t, "push rejected", *failed.Error)

		retried := mustMove(ctx, t, repo, approved.ID, pipeline.StatusFailed, pipeline.StatusApproved)
		assert.Nil(t, retried.Error)
		assert.Equal(t, pipeline.StatusApproved, retried.Status)
	})

	t.Run("expired moves pending", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))

		moved := mustMove(ctx, t, repo, created.ID, pipeline.StatusPending, pipeline.StatusExpired)
		assert.Equal(t, pipeline.StatusExpired, moved.Status)
		assert.Nil(t, moved.DecidedAt)
	})

	t.Run("records event with note", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))
		note := "looks good"
		_, err := repo.Transition(ctx, pipeline.Transition{
			ID: created.ID, From: pipeline.StatusPending, To: pipeline.StatusApproved, Actor: "marco", Note: &note,
		})
		require.NoError(t, err)

		events, err := repo.Events(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, "marco", events[1].Actor)
		require.NotNil(t, events[1].Note)
		assert.Equal(t, "looks good", *events[1].Note)
	})

	t.Run("wrong from status is conflict", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))

		_, err := repo.Transition(ctx, pipeline.Transition{
			ID: created.ID, From: pipeline.StatusApproved, To: pipeline.StatusApplying, Actor: "tester",
		})
		require.ErrorIs(t, err, domain.ErrConflict)

		got, err := repo.Get(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, pipeline.StatusPending, got.Status)
	})

	t.Run("missing id is not found", func(t *testing.T) {
		repo := setup(ctx, t)

		_, err := repo.Transition(ctx, pipeline.Transition{
			ID: "00000000-0000-0000-0000-000000000000", From: pipeline.StatusPending, To: pipeline.StatusApproved, Actor: "tester",
		})
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestReplaceFiles(t *testing.T) {
	ctx := t.Context()

	t.Run("replaces files and records revised event", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))
		files := []pipeline.File{{TargetPath: "global/skills/alpha/NEW.md", Content: "new"}}

		replaced, err := repo.ReplaceFiles(ctx, created.ID, "marco", files)
		require.NoError(t, err)
		assert.Equal(t, files, replaced.Files)
		assert.Equal(t, pipeline.StatusPending, replaced.Status)
		assert.False(t, replaced.UpdatedAt.Before(created.UpdatedAt))

		events, err := repo.Events(ctx, created.ID)
		require.NoError(t, err)
		require.Len(t, events, 2)
		require.NotNil(t, events[1].FromStatus)
		assert.Equal(t, pipeline.StatusPending, *events[1].FromStatus)
		assert.Equal(t, pipeline.StatusPending, events[1].ToStatus)
		assert.Equal(t, "marco", events[1].Actor)
		require.NotNil(t, events[1].Note)
		assert.Equal(t, "revised", *events[1].Note)
	})

	t.Run("keeping the same target is not a conflict with itself", func(t *testing.T) {
		repo := setup(ctx, t)
		created := mustCreate(ctx, t, repo, skillProposal("alpha"))
		files := []pipeline.File{{TargetPath: "global/skills/alpha/SKILL.md", Content: "revised"}}

		replaced, err := repo.ReplaceFiles(ctx, created.ID, "marco", files)
		require.NoError(t, err)
		assert.Equal(t, files, replaced.Files)
	})

	t.Run("missing id is not found", func(t *testing.T) {
		repo := setup(ctx, t)

		_, err := repo.ReplaceFiles(ctx, "00000000-0000-0000-0000-000000000000", "marco", skillProposal("alpha").Files)
		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("not pending is conflict", func(t *testing.T) {
		repo := setup(ctx, t)
		approved := mustApprove(ctx, t, repo, skillProposal("alpha"))

		_, err := repo.ReplaceFiles(ctx, approved.ID, "marco", skillProposal("alpha").Files)
		require.ErrorIs(t, err, domain.ErrConflict)
	})

	t.Run("target held by another open proposal is conflict", func(t *testing.T) {
		repo := setup(ctx, t)
		mustCreate(ctx, t, repo, skillProposal("alpha"))
		other := mustCreate(ctx, t, repo, skillProposal("beta"))

		_, err := repo.ReplaceFiles(ctx, other.ID, "marco", skillProposal("alpha").Files)
		require.ErrorIs(t, err, domain.ErrConflict)

		got, err := repo.Get(ctx, other.ID)
		require.NoError(t, err)
		assert.Equal(t, skillProposal("beta").Files, got.Files)
	})
}

func TestClaim(t *testing.T) {
	ctx := t.Context()

	t.Run("none approved returns nothing to claim", func(t *testing.T) {
		repo := setup(ctx, t)
		mustCreate(ctx, t, repo, skillProposal("alpha"))

		claimed, err := repo.Claim(ctx, "worker-1", time.Minute)
		require.ErrorIs(t, err, pipeline.ErrNothingToClaim)
		assert.Equal(t, pipeline.Proposal{}, claimed)
	})

	t.Run("takes oldest decided first and sets lease", func(t *testing.T) {
		repo := setup(ctx, t)
		newer := mustApprove(ctx, t, repo, skillProposal("alpha"))
		older := mustApprove(ctx, t, repo, skillProposal("beta"))
		backdateDecided(ctx, t, older.ID, time.Hour)

		claimed, err := repo.Claim(ctx, "worker-1", time.Minute)
		require.NoError(t, err)
		assert.Equal(t, older.ID, claimed.ID)
		assert.Equal(t, pipeline.StatusApplying, claimed.Status)
		require.NotNil(t, claimed.ClaimedBy)
		assert.Equal(t, "worker-1", *claimed.ClaimedBy)
		require.NotNil(t, claimed.ClaimedUntil)
		assert.True(t, claimed.ClaimedUntil.After(time.Now()))
		assert.Len(t, claimed.Files, 1)

		next, err := repo.Claim(ctx, "worker-2", time.Minute)
		require.NoError(t, err)
		assert.Equal(t, newer.ID, next.ID)

		events, err := repo.Events(ctx, older.ID)
		require.NoError(t, err)
		require.Len(t, events, 3)
		assert.Equal(t, pipeline.StatusApplying, events[2].ToStatus)
		assert.Equal(t, "worker-1", events[2].Actor)
	})

	t.Run("concurrent claims of one approved row give it to exactly one", func(t *testing.T) {
		repo := setup(ctx, t)
		approved := mustApprove(ctx, t, repo, skillProposal("alpha"))

		results := make([]pipeline.Proposal, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = repo.Claim(ctx, "worker", time.Minute)
			}()
		}
		wg.Wait()

		claimed := 0
		for i, result := range results {
			if errors.Is(errs[i], pipeline.ErrNothingToClaim) {
				continue
			}
			require.NoError(t, errs[i])
			claimed++
			assert.Equal(t, approved.ID, result.ID)
		}
		assert.Equal(t, 1, claimed)
	})

	t.Run("expired lease is reset and reclaimed", func(t *testing.T) {
		repo := setup(ctx, t)
		approved := mustApprove(ctx, t, repo, skillProposal("alpha"))

		first, err := repo.Claim(ctx, "worker-1", time.Second)
		require.NoError(t, err)
		assert.Equal(t, approved.ID, first.ID)

		time.Sleep(1200 * time.Millisecond)

		second, err := repo.Claim(ctx, "worker-2", time.Minute)
		require.NoError(t, err)
		assert.Equal(t, approved.ID, second.ID)
		require.NotNil(t, second.ClaimedBy)
		assert.Equal(t, "worker-2", *second.ClaimedBy)

		events, err := repo.Events(ctx, approved.ID)
		require.NoError(t, err)
		require.Len(t, events, 5)
		assert.Equal(t, "lease-expired", events[3].Actor)
		assert.Equal(t, pipeline.StatusApproved, events[3].ToStatus)
		assert.Equal(t, pipeline.StatusApplying, events[4].ToStatus)
	})
}

func TestExpirePending(t *testing.T) {
	ctx := t.Context()

	t.Run("expires only old pending proposals", func(t *testing.T) {
		repo := setup(ctx, t)
		oldPending := mustCreate(ctx, t, repo, skillProposal("alpha"))
		freshPending := mustCreate(ctx, t, repo, skillProposal("beta"))
		oldApproved := mustApprove(ctx, t, repo, skillProposal("gamma"))
		backdateCreated(ctx, t, oldPending.ID, 48*time.Hour)
		backdateCreated(ctx, t, oldApproved.ID, 48*time.Hour)

		count, err := repo.ExpirePending(ctx, 24*time.Hour, "janitor")
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		got, err := repo.Get(ctx, oldPending.ID)
		require.NoError(t, err)
		assert.Equal(t, pipeline.StatusExpired, got.Status)

		got, err = repo.Get(ctx, freshPending.ID)
		require.NoError(t, err)
		assert.Equal(t, pipeline.StatusPending, got.Status)

		got, err = repo.Get(ctx, oldApproved.ID)
		require.NoError(t, err)
		assert.Equal(t, pipeline.StatusApproved, got.Status)

		events, err := repo.Events(ctx, oldPending.ID)
		require.NoError(t, err)
		require.Len(t, events, 2)
		assert.Equal(t, pipeline.StatusExpired, events[1].ToStatus)
		assert.Equal(t, "janitor", events[1].Actor)
	})

	t.Run("nothing to expire returns zero", func(t *testing.T) {
		repo := setup(ctx, t)
		mustCreate(ctx, t, repo, skillProposal("alpha"))

		count, err := repo.ExpirePending(ctx, 24*time.Hour, "janitor")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestRejections(t *testing.T) {
	ctx := t.Context()

	t.Run("empty is empty slice", func(t *testing.T) {
		repo := setup(ctx, t)

		got, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("filters by kind and orders newest first", func(t *testing.T) {
		repo := setup(ctx, t)
		skill := mustCreate(ctx, t, repo, skillProposal("alpha"))
		rule := mustCreate(ctx, t, repo, ruleProposal("beta"))
		mustMove(ctx, t, repo, skill.ID, pipeline.StatusPending, pipeline.StatusRejected)
		mustMove(ctx, t, repo, rule.ID, pipeline.StatusPending, pipeline.StatusRejected)

		all, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, all, 2)
		assert.Equal(t, pipeline.KindRule, all[0].Kind)
		assert.Equal(t, pipeline.KindSkill, all[1].Kind)

		kind := pipeline.KindSkill
		filtered, err := repo.Rejections(ctx, &kind)
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		assert.Equal(t, "alpha", filtered[0].Name)
	})
}

func TestUpsertRejection(t *testing.T) {
	ctx := t.Context()

	t.Run("inserts a rejection visible via Rejections", func(t *testing.T) {
		repo := setup(ctx, t)
		reason := "noisy"

		require.NoError(t, repo.UpsertRejection(ctx, pipeline.Rejection{Kind: pipeline.KindRule, Name: "go", Reason: &reason}))

		got, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, pipeline.KindRule, got[0].Kind)
		assert.Equal(t, "go", got[0].Name)
		require.NotNil(t, got[0].Reason)
		assert.Equal(t, "noisy", *got[0].Reason)
	})

	t.Run("upsert overwrites the reason", func(t *testing.T) {
		repo := setup(ctx, t)
		first := "first"
		second := "second"
		require.NoError(t, repo.UpsertRejection(ctx, pipeline.Rejection{Kind: pipeline.KindRule, Name: "go", Reason: &first}))

		require.NoError(t, repo.UpsertRejection(ctx, pipeline.Rejection{Kind: pipeline.KindRule, Name: "go", Reason: &second}))

		got, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].Reason)
		assert.Equal(t, "second", *got[0].Reason)
	})

	t.Run("nil reason is stored as null", func(t *testing.T) {
		repo := setup(ctx, t)

		require.NoError(t, repo.UpsertRejection(ctx, pipeline.Rejection{Kind: pipeline.KindSkill, Name: "alpha"}))

		got, err := repo.Rejections(ctx, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Nil(t, got[0].Reason)
	})
}
