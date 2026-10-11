package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

type fakePipeline struct {
	t             *testing.T
	mu            sync.Mutex
	claims        []pipeline.Proposal
	applied       map[string]string
	failed        map[string]string
	created       []client.CreateRequest
	createStatus  int
	createMessage string
	onApplied     func(id string)
	rejections    []client.AddRejectionRequest
}

func newFakePipeline(t *testing.T) (*fakePipeline, *client.Client) {
	t.Helper()

	fake := &fakePipeline{t: t, applied: map[string]string{}, failed: map[string]string{}, createStatus: http.StatusCreated}
	srv := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(srv.Close)

	t.Setenv("PIPELINE_URL", srv.URL)
	t.Setenv("PIPELINE_TOKEN", "tok")

	return fake, client.New(client.Config{URL: srv.URL, Token: "tok"}, srv.Client())
}

func (f *fakePipeline) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.URL.Path == "/v1/proposals/claim":
		if len(f.claims) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next := f.claims[0]
		f.claims = f.claims[1:]
		assert.NoError(f.t, json.NewEncoder(w).Encode(next))
	case r.URL.Path == "/v1/proposals" && r.Method == http.MethodPost:
		var req client.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = append(f.created, req)
		if f.createStatus != http.StatusCreated {
			w.WriteHeader(f.createStatus)
			assert.NoError(f.t, json.NewEncoder(w).Encode(map[string]string{"detail": f.createMessage}))
			return
		}
		w.WriteHeader(http.StatusCreated)
		assert.NoError(f.t, json.NewEncoder(w).Encode(pipeline.Proposal{ID: "new-" + pipeline.GroupOf(req.Files[0].TargetPath)}))
	case r.URL.Path == "/v1/rejections" && r.Method == http.MethodPost:
		var req client.AddRejectionRequest
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&req))
		f.rejections = append(f.rejections, req)
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(r.URL.Path, "/applied"):
		var body struct {
			SHA string `json:"sha"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.applied[proposalID(r.URL.Path)] = body.SHA
		if f.onApplied != nil {
			f.onApplied(proposalID(r.URL.Path))
		}
		assert.NoError(f.t, json.NewEncoder(w).Encode(pipeline.Proposal{ID: proposalID(r.URL.Path)}))
	case strings.HasSuffix(r.URL.Path, "/failed"):
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.failed[proposalID(r.URL.Path)] = body.Error
		assert.NoError(f.t, json.NewEncoder(w).Encode(pipeline.Proposal{ID: proposalID(r.URL.Path)}))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func proposalID(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-2]
}

func newWorkRepo(t *testing.T) (string, string, string) {
	t.Helper()

	bare := filepath.Join(t.TempDir(), "canonical.git")
	initBareRepo(t, bare)
	commitFiles(t, bare, map[string]string{"README.md": "seed\n"}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	workDir := filepath.Join(t.TempDir(), "work")
	runGit(t, "", nil, "clone", "file://"+bare, workDir)
	runGit(t, workDir, nil, "config", "user.name", "test")
	runGit(t, workDir, nil, "config", "user.email", "test@test.local")

	return bare, workDir, filepath.Join(t.TempDir(), "lock")
}

func ruleProposal(id, name, content string) pipeline.Proposal {
	return pipeline.Proposal{
		ID:    id,
		Kind:  pipeline.KindRule,
		Name:  name,
		Files: []pipeline.File{{TargetPath: "global/rules/" + name, Content: content}},
	}
}

func TestApplyApproved(t *testing.T) {
	t.Run("commits marks applied and pushes once", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n"), ruleProposal("p2", "two.md", "two\n")}

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
		require.NoError(t, err)
		assert.Equal(t, []string{"p1", "p2"}, result.Applied)
		assert.Empty(t, result.Failed)
		assert.Equal(t, []appliedProposal{
			{ID: "p1", Kind: pipeline.KindRule, Name: "one.md", Targets: []string{"global/rules/one.md"}},
			{ID: "p2", Kind: pipeline.KindRule, Name: "two.md", Targets: []string{"global/rules/two.md"}},
		}, result.AppliedProposals)

		head := strings.TrimSpace(runGit(t, workDir, nil, "rev-parse", "HEAD"))
		assert.Equal(t, head, strings.TrimSpace(runGit(t, bare, nil, "rev-parse", "main")))
		assert.Equal(t, head, fake.applied["p2"])
		assert.NotEqual(t, head, fake.applied["p1"])
		assert.NotEmpty(t, fake.applied["p1"])

		log := runGit(t, workDir, nil, "log", "--format=%s")
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

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
		require.ErrorIs(t, err, errProposalsFailed)
		assert.Equal(t, []string{"good"}, result.Applied)
		assert.Equal(t, []string{"bad"}, result.Failed)
		require.Len(t, result.AppliedProposals, 1)
		assert.Equal(t, "good", result.AppliedProposals[0].ID)
		assert.Contains(t, fake.failed, "bad")
		assert.Contains(t, fake.applied, "good")

		head := strings.TrimSpace(runGit(t, workDir, nil, "rev-parse", "HEAD"))
		assert.Equal(t, head, strings.TrimSpace(runGit(t, bare, nil, "rev-parse", "main")))

		_, statErr := os.Stat(filepath.Join(workDir, "global", "rules", "bad.md"))
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("push failure marks every committed proposal failed and returns error", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		bare, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{ruleProposal("p1", "one.md", "one\n"), ruleProposal("p2", "two.md", "two\n")}
		require.NoError(t, os.Rename(bare, bare+".gone"))

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
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

		_, err := applyApproved(t.Context(), c, workDir, lock, "w1")
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
			_, applyErr := applyApproved(t.Context(), c, workDir, lock, "w1")
			done <- applyErr
		}()

		select {
		case <-done:
			t.Fatal("apply finished while the lock was held")
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
		before := strings.TrimSpace(runGit(t, bare, nil, "rev-parse", "main"))

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
		require.NoError(t, err)
		assert.Empty(t, result.Applied)
		assert.Empty(t, result.Failed)
		assert.NotNil(t, result.AppliedProposals)
		assert.Empty(t, result.AppliedProposals)
		assert.Equal(t, before, strings.TrimSpace(runGit(t, bare, nil, "rev-parse", "main")))
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

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
		require.ErrorIs(t, err, errProposalsFailed)
		assert.Equal(t, []string{"evil"}, result.Failed)
		assert.Contains(t, fake.failed["evil"], "unsafe")
	})

	t.Run("proposal without files is marked failed", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		_, workDir, lock := newWorkRepo(t)
		fake.claims = []pipeline.Proposal{{ID: "empty", Kind: pipeline.KindRule, Name: "x.md"}}

		result, err := applyApproved(t.Context(), c, workDir, lock, "w1")
		require.ErrorIs(t, err, errProposalsFailed)
		assert.Equal(t, []string{"empty"}, result.Failed)
	})
}

func TestImportProposals(t *testing.T) {
	t.Run("groups files by proposal and uses dir basename as source run", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		dir := filepath.Join(t.TempDir(), "job-123")
		require.NoError(t, os.MkdirAll(dir, 0o750))
		writeStaged(t, dir, "global__skills__foo__SKILL.md", "global/skills/foo/SKILL.md", "skill\n")
		writeStaged(t, dir, "global__skills__foo__extra.md", "global/skills/foo/extra.md", "extra\n")
		writeStaged(t, dir, "global__rules__r.md", "global/rules/r.md", "rule\n")

		outcome, err := importProposals(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Equal(t, []string{"new-r.md", "new-foo"}, outcome.Created)
		assert.Empty(t, outcome.Rejected)
		require.Len(t, fake.created, 2)
		for _, req := range fake.created {
			assert.Equal(t, "taskmine", req.SourceFlow)
			assert.Equal(t, "job-123", req.SourceRun)
		}
		assert.Len(t, fake.created[0].Files, 1)
		assert.Len(t, fake.created[1].Files, 2)
	})

	t.Run("409 becomes rejected entry", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		fake.createStatus = http.StatusConflict
		fake.createMessage = "open proposal exists"
		dir := t.TempDir()
		writeStaged(t, dir, "global__rules__r.md", "global/rules/r.md", "rule\n")

		outcome, err := importProposals(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		assert.Equal(t, []rejectedOp{{Name: "r.md", Reason: "open proposal exists"}}, outcome.Rejected)
	})

	t.Run("invalid target is rejected without calling the api", func(t *testing.T) {
		fake, c := newFakePipeline(t)
		dir := t.TempDir()
		writeStaged(t, dir, "evil.md", "global/../evil.md", "x")

		outcome, err := importProposals(t.Context(), c, dir, "taskmine")
		require.NoError(t, err)
		assert.Empty(t, outcome.Created)
		require.Len(t, outcome.Rejected, 1)
		assert.Equal(t, "evil.md", outcome.Rejected[0].Name)
		assert.Empty(t, fake.created)
	})

	t.Run("missing directory fails", func(t *testing.T) {
		_, c := newFakePipeline(t)

		_, err := importProposals(t.Context(), c, filepath.Join(t.TempDir(), "nope"), "taskmine")
		require.Error(t, err)
	})
}

func TestCmdProposalImportRejections(t *testing.T) {
	t.Run("infers kinds dedupes and skips empty lines", func(t *testing.T) {
		fake, _ := newFakePipeline(t)
		file := filepath.Join(t.TempDir(), "taskmine-rejected")
		content := "my-skill\n\n  bug-login.md \ntask-x.md\ntodo-y.md\nnotes.md\nmy-skill\n\n"
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))

		err := cmdProposal(t.Context(), []string{"import-rejections", "--file", file, "--actor", "migrator"})
		require.NoError(t, err)

		reason := "imported from taskmine-rejected"
		assert.Equal(t, []client.AddRejectionRequest{
			{Kind: pipeline.KindSkill, Name: "my-skill", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindBug, Name: "bug-login.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindOneTimeTask, Name: "task-x.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindCodeChange, Name: "todo-y.md", Actor: "migrator", Reason: &reason},
			{Kind: pipeline.KindProjectEntry, Name: "notes.md", Actor: "migrator", Reason: &reason},
		}, fake.rejections)
	})

	t.Run("missing file flag is a validation error", func(t *testing.T) {
		newFakePipeline(t)

		err := cmdProposal(t.Context(), []string{"import-rejections", "--actor", "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("nonexistent file is a validation error", func(t *testing.T) {
		newFakePipeline(t)

		err := cmdProposal(t.Context(), []string{"import-rejections", "--file", filepath.Join(t.TempDir(), "nope"), "--actor", "a"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func TestGroupFiles(t *testing.T) {
	t.Run("skill files share a group and keep first-seen order", func(t *testing.T) {
		groups := groupFiles([]pipeline.File{
			{TargetPath: "global/rules/a.md"},
			{TargetPath: "global/skills/s/SKILL.md"},
			{TargetPath: "global/skills/s/ref.md"},
		})

		require.Len(t, groups, 2)
		assert.Equal(t, "a.md", groups[0].Name)
		assert.Equal(t, "s", groups[1].Name)
		assert.Len(t, groups[1].Files, 2)
	})

	t.Run("no files yields no groups", func(t *testing.T) {
		assert.Empty(t, groupFiles(nil))
	})
}

func TestCmdProposal(t *testing.T) {
	t.Run("missing subcommand is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), nil)
		require.ErrorIs(t, err, errUnknownSubcommand)
	})

	t.Run("unknown subcommand is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), []string{"nope"})
		require.ErrorIs(t, err, errUnknownSubcommand)
	})

	t.Run("missing env fails with validation error", func(t *testing.T) {
		t.Setenv("PIPELINE_URL", "")
		t.Setenv("PIPELINE_TOKEN", "")

		err := cmdProposal(t.Context(), []string{"get", "--id", "p1"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})

	t.Run("unknown status filter is rejected", func(t *testing.T) {
		err := cmdProposal(t.Context(), []string{"list", "--status", "bogus"})
		require.ErrorIs(t, err, domain.ErrValidation)
	})
}

func writeStaged(t *testing.T, dir, flattened, target, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, flattened), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, flattened+".path"), []byte(target), 0o600))
}
