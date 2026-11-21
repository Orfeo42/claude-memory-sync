package synthapi

import (
	"net/http"
	"testing"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/footprint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFootprint(t *testing.T) {
	t.Run("passes work dir and returns report with empty lists", func(t *testing.T) {
		var gotDir string
		fn := func(workDir string, now time.Time) (footprint.Report, error) {
			gotDir = workDir
			return footprint.Report{GeneratedAt: now, Limits: footprint.Limits{MemoryLines: 200, MemoryBytes: 25600, WarnPct: 80}}, nil
		}
		rec := do(t, New(Deps{Footprint: fn, WorkDir: "/work"}), http.MethodGet, "/api/report/footprint", "")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "/work", gotDir)
		assert.Contains(t, rec.Body.String(), `"projects":[]`)
		assert.Contains(t, rec.Body.String(), `"rules_scoped":[]`)
		assert.Contains(t, rec.Body.String(), `"rules_unconditional":[]`)
		assert.Contains(t, rec.Body.String(), `"memory_lines":200`)
	})

	t.Run("upstream error is 502", func(t *testing.T) {
		fn := func(_ string, _ time.Time) (footprint.Report, error) {
			return footprint.Report{}, domain.Error(domain.ErrUpstream, "down")
		}
		rec := do(t, New(Deps{Footprint: fn}), http.MethodGet, "/api/report/footprint", "")
		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("own error is 500", func(t *testing.T) {
		fn := func(_ string, _ time.Time) (footprint.Report, error) { return footprint.Report{}, assert.AnError }
		rec := do(t, New(Deps{Footprint: fn}), http.MethodGet, "/api/report/footprint", "")
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
