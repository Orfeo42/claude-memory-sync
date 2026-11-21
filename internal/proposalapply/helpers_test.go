package proposalapply

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"claude-memory-sync/internal/gitutil/gittest"
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
	gittest.InitBareRepo(t, bare)
	gittest.CommitFiles(t, bare, map[string]string{"README.md": "seed\n"}, "seed", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	workDir := filepath.Join(t.TempDir(), "work")
	gittest.RunGit(t, "", nil, "clone", "file://"+bare, workDir)
	gittest.RunGit(t, workDir, nil, "config", "user.name", "test")
	gittest.RunGit(t, workDir, nil, "config", "user.email", "test@test.local")

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

func writeStaged(t *testing.T, dir, flattened, target, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, flattened), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, flattened+".path"), []byte(target), 0o600))
}
