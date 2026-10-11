package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/httpapi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	writerToken = "writer-token"
	adminToken  = "admin-token"
	rulePath    = "global/rules/go.md"
)

func newHandler(repo *pipeline.MockRepository) http.Handler {
	return httpapi.New(context.Background(), pipeline.NewService(repo), writerToken, adminToken)
}

func do(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func transitionRepo() *pipeline.MockRepository {
	return &pipeline.MockRepository{
		TransitionFunc: func(_ context.Context, tr pipeline.Transition) (pipeline.Proposal, error) {
			return pipeline.Proposal{ID: tr.ID, Status: tr.To}, nil
		},
	}
}

func conflictRepo() *pipeline.MockRepository {
	return &pipeline.MockRepository{
		TransitionFunc: func(_ context.Context, _ pipeline.Transition) (pipeline.Proposal, error) {
			return pipeline.Proposal{}, domain.Error(domain.ErrConflict, "wrong status")
		},
	}
}

func TestHealthz(t *testing.T) {
	t.Run("requires no auth", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/healthz", "", "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestAuth(t *testing.T) {
	h := newHandler(&pipeline.MockRepository{
		ListFunc:       func(_ context.Context, _ pipeline.ListFilter) ([]pipeline.Proposal, error) { return nil, nil },
		TransitionFunc: transitionRepo().TransitionFunc,
	})
	decision := `{"decision":"approve","actor":"a"}`

	t.Run("writer route without token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/proposals", "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("writer route with wrong token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/proposals", "nope", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("writer route accepts writer token", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/proposals", writerToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("writer route accepts admin token", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/v1/proposals", adminToken, "")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("admin route without token is 401", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/proposals/p1/decision", "", decision)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("admin route with writer token is 403", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/proposals/p1/decision", writerToken, decision)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("admin route with admin token passes", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/v1/proposals/p1/decision", adminToken, decision)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("revise and retry are admin only", func(t *testing.T) {
		rec := do(t, h, http.MethodPut, "/v1/proposals/p1/files", writerToken, `{"actor":"a","files":[{"target_path":"`+rulePath+`","content":"x"}]}`)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		rec = do(t, h, http.MethodPost, "/v1/proposals/p1/retry", writerToken, `{"actor":"a"}`)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestCreateProposal(t *testing.T) {
	body := `{"source_flow":"f","source_run":"r","files":[{"target_path":"` + rulePath + `","content":"x"}]}`

	t.Run("returns 201 with proposal", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			CreateFunc: func(_ context.Context, p pipeline.NewProposal) (pipeline.Proposal, error) {
				return pipeline.Proposal{ID: "p1", Kind: p.Kind, Name: p.Name, Status: pipeline.StatusPending}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals", writerToken, body)

		require.Equal(t, http.StatusCreated, rec.Code)
		var got pipeline.Proposal
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "p1", got.ID)
		assert.Equal(t, pipeline.KindRule, got.Kind)
	})

	t.Run("invalid target is 400", func(t *testing.T) {
		bad := `{"source_flow":"f","source_run":"r","files":[{"target_path":"other/x.md","content":"x"}]}`

		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals", writerToken, bad)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("open proposal on target is 409", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			CreateFunc: func(_ context.Context, _ pipeline.NewProposal) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, domain.Error(domain.ErrConflict, "open proposal on target")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals", writerToken, body)

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Contains(t, rec.Body.String(), "open proposal on target")
	})

	t.Run("empty files rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals", writerToken, `{"source_flow":"f","source_run":"r","files":[]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty source_flow rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals", writerToken, `{"source_flow":"","source_run":"r","files":[{"target_path":"`+rulePath+`","content":"x"}]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("repository failure is 500", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			CreateFunc: func(_ context.Context, _ pipeline.NewProposal) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, domain.Error(domain.ErrInternal, "db down")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals", writerToken, body)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("upstream failure is 502", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			CreateFunc: func(_ context.Context, _ pipeline.NewProposal) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, domain.Error(domain.ErrUpstream, "db unreachable")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals", writerToken, body)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestListProposals(t *testing.T) {
	t.Run("returns proposals wrapper and default limit", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ListFunc: func(_ context.Context, _ pipeline.ListFilter) ([]pipeline.Proposal, error) {
				return []pipeline.Proposal{{ID: "a"}}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/proposals", writerToken, "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"proposals"`)
		assert.Equal(t, 100, repo.ListCalls()[0].F.Limit)
		assert.Nil(t, repo.ListCalls()[0].F.Status)
	})

	t.Run("empty result serializes as empty array", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ListFunc: func(_ context.Context, _ pipeline.ListFilter) ([]pipeline.Proposal, error) { return nil, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/proposals", writerToken, "")

		assert.Contains(t, rec.Body.String(), `"proposals":[]`)
	})

	t.Run("passes status kind before and limit filters", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ListFunc: func(_ context.Context, _ pipeline.ListFilter) ([]pipeline.Proposal, error) { return nil, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/proposals?status=pending&kind=skill&before=2026-01-02T03:04:05Z&limit=5", writerToken, "")

		require.Equal(t, http.StatusOK, rec.Code)
		f := repo.ListCalls()[0].F
		assert.Equal(t, pipeline.StatusPending, *f.Status)
		assert.Equal(t, pipeline.KindSkill, *f.Kind)
		assert.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), f.Before.UTC())
		assert.Equal(t, 5, f.Limit)
	})

	t.Run("unknown status rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/proposals?status=bogus", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("unknown kind rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/proposals?kind=bogus", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("limit above 500 rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/proposals?limit=501", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("limit below 1 rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/proposals?limit=0", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("malformed before rejected", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/proposals?before=yesterday", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestGetProposal(t *testing.T) {
	t.Run("returns proposal and events", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			GetFunc:    func(_ context.Context, id string) (pipeline.Proposal, error) { return pipeline.Proposal{ID: id}, nil },
			EventsFunc: func(_ context.Context, _ string) ([]pipeline.Event, error) { return []pipeline.Event{{ID: 1}}, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/proposals/p1", writerToken, "")

		require.Equal(t, http.StatusOK, rec.Code)
		var got pipeline.ProposalDetail
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "p1", got.Proposal.ID)
		assert.Len(t, got.Events, 1)
	})

	t.Run("missing id is 404", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			GetFunc: func(_ context.Context, _ string) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, domain.Error(domain.ErrNotFound, "proposal not found")
			},
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/proposals/p1", writerToken, "")

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestDecision(t *testing.T) {
	t.Run("approve returns approved proposal", func(t *testing.T) {
		repo := transitionRepo()

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/p1/decision", adminToken, `{"decision":"approve","actor":"alice"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"approved"`)
		assert.Equal(t, "alice", repo.TransitionCalls()[0].T.Actor)
	})

	t.Run("reject carries reason as note", func(t *testing.T) {
		repo := transitionRepo()

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/p1/decision", adminToken, `{"decision":"reject","actor":"alice","reason":"no"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "no", *repo.TransitionCalls()[0].T.Note)
	})

	t.Run("not pending is 409", func(t *testing.T) {
		rec := do(t, newHandler(conflictRepo()), http.MethodPost, "/v1/proposals/p1/decision", adminToken, `{"decision":"approve","actor":"alice"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("unknown decision rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(transitionRepo()), http.MethodPost, "/v1/proposals/p1/decision", adminToken, `{"decision":"maybe","actor":"alice"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty actor rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(transitionRepo()), http.MethodPost, "/v1/proposals/p1/decision", adminToken, `{"decision":"approve","actor":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestReviseFiles(t *testing.T) {
	body := `{"actor":"alice","files":[{"target_path":"` + rulePath + `","content":"new"}]}`

	t.Run("returns revised proposal", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			GetFunc: func(_ context.Context, _ string) (pipeline.Proposal, error) {
				return pipeline.Proposal{Name: "go.md"}, nil
			},
			ReplaceFilesFunc: func(_ context.Context, id, _ string, files []pipeline.File) (pipeline.Proposal, error) {
				return pipeline.Proposal{ID: id, Files: files}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPut, "/v1/proposals/p1/files", adminToken, body)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"new"`)
	})

	t.Run("group change is 400", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			GetFunc: func(_ context.Context, _ string) (pipeline.Proposal, error) {
				return pipeline.Proposal{Name: "other.md"}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPut, "/v1/proposals/p1/files", adminToken, body)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("not pending is 409", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			GetFunc: func(_ context.Context, _ string) (pipeline.Proposal, error) {
				return pipeline.Proposal{Name: "go.md"}, nil
			},
			ReplaceFilesFunc: func(_ context.Context, _, _ string, _ []pipeline.File) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, domain.Error(domain.ErrConflict, "not pending")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPut, "/v1/proposals/p1/files", adminToken, body)

		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("empty files rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPut, "/v1/proposals/p1/files", adminToken, `{"actor":"a","files":[]}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestRetry(t *testing.T) {
	t.Run("returns approved proposal", func(t *testing.T) {
		rec := do(t, newHandler(transitionRepo()), http.MethodPost, "/v1/proposals/p1/retry", adminToken, `{"actor":"alice"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"approved"`)
	})

	t.Run("not failed is 409", func(t *testing.T) {
		rec := do(t, newHandler(conflictRepo()), http.MethodPost, "/v1/proposals/p1/retry", adminToken, `{"actor":"alice"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestClaim(t *testing.T) {
	t.Run("returns 200 with claimed proposal and lease", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ClaimFunc: func(_ context.Context, _ string, _ time.Duration) (pipeline.Proposal, error) {
				return pipeline.Proposal{ID: "p1", Status: pipeline.StatusApplying}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/claim", writerToken, `{"worker":"w","lease_seconds":60}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"p1"`)
		assert.Equal(t, time.Minute, repo.ClaimCalls()[0].Lease)
	})

	t.Run("returns 204 without body when nothing claimable", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ClaimFunc: func(_ context.Context, _ string, _ time.Duration) (pipeline.Proposal, error) {
				return pipeline.Proposal{}, pipeline.ErrNothingToClaim
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/claim", writerToken, `{"worker":"w"}`)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
		assert.Equal(t, 10*time.Minute, repo.ClaimCalls()[0].Lease)
	})

	t.Run("lease above 3600 rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals/claim", writerToken, `{"worker":"w","lease_seconds":3601}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty worker rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals/claim", writerToken, `{"worker":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestApplied(t *testing.T) {
	t.Run("returns applied proposal with sha", func(t *testing.T) {
		repo := transitionRepo()

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/p1/applied", writerToken, `{"actor":"w","sha":"abc"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "abc", *repo.TransitionCalls()[0].T.AppliedSHA)
	})

	t.Run("not applying is 409", func(t *testing.T) {
		rec := do(t, newHandler(conflictRepo()), http.MethodPost, "/v1/proposals/p1/applied", writerToken, `{"actor":"w","sha":"abc"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("empty sha rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(transitionRepo()), http.MethodPost, "/v1/proposals/p1/applied", writerToken, `{"actor":"w","sha":""}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestFailed(t *testing.T) {
	t.Run("returns failed proposal with error note", func(t *testing.T) {
		repo := transitionRepo()

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/p1/failed", writerToken, `{"actor":"w","error":"boom"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "boom", *repo.TransitionCalls()[0].T.Note)
	})

	t.Run("not applying is 409", func(t *testing.T) {
		rec := do(t, newHandler(conflictRepo()), http.MethodPost, "/v1/proposals/p1/failed", writerToken, `{"actor":"w","error":"boom"}`)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestExpire(t *testing.T) {
	t.Run("returns expired count", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ExpirePendingFunc: func(_ context.Context, _ time.Duration, _ string) (int, error) { return 4, nil },
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/expire", writerToken, `{"actor":"cron","older_than_hours":48}`)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"expired":4}`, stripSchema(t, rec.Body.String()))
		assert.Equal(t, 48*time.Hour, repo.ExpirePendingCalls()[0].OlderThan)
	})

	t.Run("zero hours rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/proposals/expire", writerToken, `{"actor":"cron","older_than_hours":0}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("repository failure is 500", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			ExpirePendingFunc: func(_ context.Context, _ time.Duration, _ string) (int, error) {
				return 0, domain.Error(domain.ErrInternal, "db")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/proposals/expire", writerToken, `{"actor":"cron","older_than_hours":1}`)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestRejections(t *testing.T) {
	t.Run("returns rejections wrapper and passes kind", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			RejectionsFunc: func(_ context.Context, _ *pipeline.Kind) ([]pipeline.Rejection, error) {
				return []pipeline.Rejection{{Name: "x"}}, nil
			},
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/rejections?kind=rule", writerToken, "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"rejections"`)
		assert.Equal(t, pipeline.KindRule, *repo.RejectionsCalls()[0].Kind)
	})

	t.Run("no kind passes nil and empty result is an array", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			RejectionsFunc: func(_ context.Context, _ *pipeline.Kind) ([]pipeline.Rejection, error) { return nil, nil },
		}

		rec := do(t, newHandler(repo), http.MethodGet, "/v1/rejections", writerToken, "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Nil(t, repo.RejectionsCalls()[0].Kind)
		assert.Contains(t, rec.Body.String(), `"rejections":[]`)
	})

	t.Run("unknown kind rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodGet, "/v1/rejections?kind=bogus", writerToken, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})
}

func TestAddRejection(t *testing.T) {
	body := `{"kind":"rule","name":"go","actor":"alice","reason":"noisy"}`

	t.Run("admin token records rejection and returns 204", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			UpsertRejectionFunc: func(_ context.Context, _ pipeline.Rejection) error { return nil },
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/rejections", adminToken, body)

		require.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
		calls := repo.UpsertRejectionCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, pipeline.KindRule, calls[0].R.Kind)
		assert.Equal(t, "go", calls[0].R.Name)
		assert.Equal(t, "noisy", *calls[0].R.Reason)
	})

	t.Run("writer token is 403", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/rejections", writerToken, body)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("no token is 401", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/rejections", "", body)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("bad kind rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/rejections", adminToken, `{"kind":"bogus","name":"go","actor":"alice"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("empty name rejected by annotation", func(t *testing.T) {
		rec := do(t, newHandler(&pipeline.MockRepository{}), http.MethodPost, "/v1/rejections", adminToken, `{"kind":"rule","name":"","actor":"alice"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	})

	t.Run("service validation error is 400", func(t *testing.T) {
		repo := &pipeline.MockRepository{
			UpsertRejectionFunc: func(_ context.Context, _ pipeline.Rejection) error {
				return domain.Error(domain.ErrValidation, "bad")
			},
		}

		rec := do(t, newHandler(repo), http.MethodPost, "/v1/rejections", adminToken, body)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func stripSchema(t *testing.T, body string) string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &m))
	delete(m, "$schema")
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return string(out)
}
