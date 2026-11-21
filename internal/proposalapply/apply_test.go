package proposalapply

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
	"claude-memory-sync/internal/pipeline"
)

func TestApplyApproved(t *testing.T) {
	t.Run("commits marks applied and pushes once", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n"), ruleProposal("p2", "two.md", "two\n")}

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.NoError(t, err)
		assert.Equal(t, []string{"p1", "p2"}, result.Applied)
		assert.Empty(t, result.Failed)
		assert.Equal(t, []AppliedProposal{
			{ID: "p1", Kind: pipeline.KindRule, Name: "one.md", Targets: []string{"global/rules/one.md"}},
			{ID: "p2", Kind: pipeline.KindRule, Name: "two.md", Targets: []string{"global/rules/two.md"}},
		}, result.AppliedProposals)

		head := strings.TrimSpace(gittest.RunGit(t, workDir, nil, "rev-parse", "HEAD"))
		assert.Equal(t, head, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")))
		assert.Equal(t, head, fake.applied["p2"])
		assert.NotEqual(t, head, fake.applied["p1"])
		assert.NotEmpty(t, fake.applied["p1"])

		log := gittest.RunGit(t, workDir, nil, "log", "--format=%s")
		assert.Contains(t, log, "proposal: apply rule one.md")
		assert.Contains(t, log, "proposal: apply rule two.md")

		content, err := os.ReadFile(filepath.Join(workDir, "global", "rules", "one.md"))
		require.NoError(t, err)
		assert.Equal(t, "one\n", string(content))
	})

	t.Run("failing proposal is marked failed and next one still applied", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		bad := ruleProposal("bad", "bad.md", "token ghp_abcdefghijklmnopqrstuvwxyz0123456789\n")
		fake.claims = []pipeline.Proposal{bad, ruleProposal("good", "good.md", "ok\n")}

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.ErrorIs(t, err, ErrProposalsFailed)
		assert.Equal(t, []string{"good"}, result.Applied)
		assert.Equal(t, []string{"bad"}, result.Failed)
		require.Len(t, result.AppliedProposals, 1)
		assert.Equal(t, "good", result.AppliedProposals[0].ID)
		assert.Contains(t, fake.failed, "bad")
		assert.Contains(t, fake.applied, "good")

		head := strings.TrimSpace(gittest.RunGit(t, workDir, nil, "rev-parse", "HEAD"))
		assert.Equal(t, head, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")))

		_, statErr := os.Stat(filepath.Join(workDir, "global", "rules", "bad.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("push failure marks every committed proposal failed and returns error", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n"), ruleProposal("p2", "two.md", "two\n")}
		require.NoError(t, os.Rename(bare, bare+".gone"))

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.Error(t, err)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.AppliedProposals)
		assert.Equal(t, []string{"p1", "p2"}, result.Failed)
		assert.Empty(t, fake.applied)
		assert.Contains(t, fake.failed["p1"], "push failed")
		assert.Contains(t, fake.failed["p2"], "push failed")
	})

	t.Run("mark applied happens only after the remote has the commit", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n")}
		remoteAtMark := ""
		var revParseErr error
		fake.onApplied = func(string) {
			out, err := exec.Command("git", "-C", bare, "rev-parse", "main").Output()
			revParseErr = err
			remoteAtMark = strings.TrimSpace(string(out))
		}

		_, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.NoError(t, err)
		require.NoError(t, revParseErr)
		assert.Equal(t, fake.applied["p1"], remoteAtMark)
	})

	t.Run("write and commit wait for the canonical lock", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		_, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n")}

		held, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o600)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, held.Close())
		}()
		require.NoError(t, syscall.Flock(int(held.Fd()), syscall.LOCK_EX))

		done := make(chan error, 1)
		go func() {
			_, applyErr := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
			done <- applyErr
		}()

		select {
		case doneErr := <-done:
			t.Fatalf("apply finished while the lock was held: %v", doneErr)
		case <-time.After(500 * time.Millisecond):
		}
		_, statErr := os.Stat(filepath.Join(workDir, "global", "rules", "one.md"))
		assert.True(t, os.IsNotExist(statErr))

		require.NoError(t, syscall.Flock(int(held.Fd()), syscall.LOCK_UN))

		select {
		case applyErr := <-done:
			require.NoError(t, applyErr)
		case <-time.After(15 * time.Second):
			t.Fatal("apply did not finish after the lock was released")
		}
	})

	t.Run("nothing to claim does not push", func(t *testing.T) {
		_, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		before := strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main"))

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.NoError(t, err)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.Failed)
		assert.NotNil(t, result.AppliedProposals)
		assert.Empty(t, result.AppliedProposals)
		assert.Equal(t, before, strings.TrimSpace(gittest.RunGit(t, bare, nil, "rev-parse", "main")))
	})

	t.Run("path traversal target is marked failed", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		_, workDir, lock := newWorkRepo(t)
		evil := pipeline.Proposal{
			ID:    "evil",
			Kind:  pipeline.KindRule,
			Name:  "passwd",
			Files: []pipeline.File{{TargetPath: "global/../../etc/passwd", Content: "x"}},
		}
		fake.claims = []pipeline.Proposal{evil}

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.ErrorIs(t, err, ErrProposalsFailed)
		assert.Equal(t, []string{"evil"}, result.Failed)
		assert.Contains(t, fake.failed["evil"], "unsafe")
	})

	t.Run("proposal without files is marked failed", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		_, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{{ID: "empty", Kind: pipeline.KindRule, Name: "x.md"}}

		result, err := ApplyApproved(t.Context(), c, ApplyApprovedOptions{WorkDir: workDir, Lock: lock, Worker: "w1"})
		require.ErrorIs(t, err, ErrProposalsFailed)
		assert.Equal(t, []string{"empty"}, result.Failed)
	})
}
