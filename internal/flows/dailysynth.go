package flows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"claude-memory-sync/internal/corpus"
	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/hotcache"
	"claude-memory-sync/internal/intake"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/proposalapply"
	"claude-memory-sync/internal/runs"
	"claude-memory-sync/internal/workrepo"
)

const (
	synthFlowName       = "daily-synth"
	synthMinEntries     = 2
	synthIndexFile      = "MEMORY.md"
	synthFenceOpen      = "```json"
	synthFenceClose     = "```"
	synthNothing        = "NOTHING"
	synthStalenessTrue  = "true"
	synthStalenessKey   = "synth-staleness"
	synthCondensePrompt = "synth-condense"
	synthFidelityPrompt = "condense-fidelity"
	synthScanPrompt     = "distill-scan"
	synthWritePrompt    = "distill-write"
	synthDigestPrompt   = "hotcache-digest"
)

const (
	errSynthJudgeFormat    = flowError("fidelity judge answer is not the expected JSON object")
	errSynthClusterInvalid = flowError("distill cluster has no title or entries")
)

type synthFidelity struct {
	Lost []string `json:"lost"`
}

type synthCluster struct {
	Title   string   `json:"title"`
	Entries []string `json:"entries"`
}

func (r *Runner) dailySynth(ctx context.Context, x *run) error {
	err := x.step(ctx, "ensure_work", func(ctx context.Context) (string, error) {
		return "", gitutil.WithFileLock(r.cfg.Lock, func() error {
			return workrepo.EnsureWork(ctx, r.cfg.ReposDir, r.cfg.WorkDir)
		})
	})
	if err != nil {
		return err
	}

	var projects []corpus.ProjectSummary
	err = x.step(ctx, "corpus_list", func(ctx context.Context) (string, error) {
		listed, err := corpus.ListProjects(ctx, r.cfg.WorkDir)
		if err != nil {
			return "", err
		}
		projects = listed
		out, err := json.Marshal(listed)
		if err != nil {
			return "", domain.Error(err, "encode corpus list failed")
		}
		return string(out), nil
	})
	if err != nil {
		return err
	}

	condenseKeys := make([]string, 0, len(projects))
	allKeys := make([]string, 0, len(projects))
	for _, p := range projects {
		allKeys = append(allKeys, p.Key)
		if p.Entries >= synthMinEntries {
			condenseKeys = append(condenseKeys, p.Key)
		}
	}
	sort.Strings(condenseKeys)
	sort.Strings(allKeys)

	if err := r.synthCondenseAll(ctx, x, condenseKeys); err != nil {
		return err
	}
	if err := r.synthDistillAll(ctx, x); err != nil {
		return err
	}
	if err := r.synthHotcacheAll(ctx, x, allKeys); err != nil {
		return err
	}

	return x.step(ctx, "push", func(ctx context.Context) (string, error) {
		return "", workrepo.Push(ctx, r.cfg.WorkDir, r.cfg.Lock)
	})
}

func (r *Runner) synthCondenseAll(ctx context.Context, x *run, keys []string) error {
	staleness, err := r.deps.Settings.Param(ctx, ParamStalenessEnabled)
	if err != nil {
		return domain.WrapError(err, slog.String("param", ParamStalenessEnabled))
	}
	for _, key := range keys {
		if err := r.synthCondenseKey(ctx, x, key, staleness == synthStalenessTrue); err != nil {
			if ctx.Err() != nil {
				return domain.Error(ctx.Err(), "daily-synth cancelled")
			}
			continue
		}
	}
	return nil
}

func (r *Runner) synthDistillAll(ctx context.Context, x *run) error {
	var scanOut string
	err := x.step(ctx, "distill_scan", func(ctx context.Context) (string, error) {
		out, err := r.synthDistillScan(ctx, x)
		scanOut = out
		return out, err
	})
	if err != nil {
		return err
	}

	for _, line := range synthClusterLines(scanOut) {
		if err := r.synthDistillCluster(ctx, x, line); err != nil {
			if ctx.Err() != nil {
				return domain.Error(ctx.Err(), "daily-synth cancelled")
			}
			continue
		}
	}
	return nil
}

func (r *Runner) synthHotcacheAll(ctx context.Context, x *run, keys []string) error {
	maxBytesRaw, err := r.deps.Settings.Param(ctx, ParamHotcacheMaxBytes)
	if err != nil {
		return domain.WrapError(err, slog.String("param", ParamHotcacheMaxBytes))
	}
	for _, key := range keys {
		if err := r.synthHotcacheKey(ctx, x, key, maxBytesRaw); err != nil {
			if ctx.Err() != nil {
				return domain.Error(ctx.Err(), "daily-synth cancelled")
			}
			continue
		}
	}
	return nil
}

func (r *Runner) synthCondenseKey(ctx context.Context, x *run, key string, staleness bool) error {
	name := "condense:" + key
	memoryDir := filepath.Join(r.cfg.WorkDir, "projects", key, "memory")

	text, lost, err := r.synthCondenseDraft(ctx, x, key, memoryDir, staleness)
	if err != nil {
		return x.step(ctx, name, func(context.Context) (string, error) { return "", err })
	}

	if len(lost) > 0 {
		encoded, err := json.Marshal(lost)
		if err != nil {
			return domain.Error(err, "encode lost facts failed", slog.String("project", key))
		}
		x.count("lossy", 1)
		return x.mark(ctx, name, runs.StepLossy, "lost: "+string(encoded))
	}

	return x.step(ctx, name, func(ctx context.Context) (string, error) {
		result, err := ops.Apply(ctx, ops.ApplyOptions{MemoryDir: memoryDir, AllowDelete: true}, strings.NewReader(text))
		if err != nil {
			return "", err
		}
		_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
			WorkDir:  r.cfg.WorkDir,
			Lock:     r.cfg.Lock,
			Message:  "synthesize: " + key,
			Pathspec: "projects/" + key + "/memory",
		})
		if err != nil {
			return "", err
		}
		x.count("condensed", 1)
		out, err := json.Marshal(result)
		if err != nil {
			return "", domain.Error(err, "encode ops result failed")
		}
		return string(out), nil
	})
}

func (r *Runner) synthCondenseDraft(ctx context.Context, x *run, key, memoryDir string, staleness bool) (string, []string, error) {
	var input bytes.Buffer
	if err := corpus.WriteContext(ctx, &input, r.cfg.ReposDir, r.cfg.WorkDir, key, intake.ScanResult{}); err != nil {
		return "", nil, err
	}

	prompt := input.String()
	if staleness {
		addendum, err := r.deps.Settings.Prompt(ctx, synthStalenessKey)
		if err != nil {
			return "", nil, domain.WrapError(err, slog.String("prompt", synthStalenessKey))
		}
		prompt = addendum + "\n\n" + prompt
	}

	text, err := x.ask(ctx, synthCondensePrompt, ParamSynthModel, prompt)
	if err != nil {
		return "", nil, err
	}
	if parsed, parseErr := ops.Parse(strings.NewReader(text)); parseErr == nil && parsed.Nothing {
		return text, nil, nil
	}

	before, err := synthReadEntries(memoryDir)
	if err != nil {
		return "", nil, err
	}

	judgeInput := fmt.Sprintf("staleness_enabled: %t\n=== BEFORE ===\n%s=== AFTER (proposed ops) ===\n%s", staleness, before, text)
	verdict, err := x.ask(ctx, synthFidelityPrompt, ParamFidelityModel, judgeInput)
	if err != nil {
		return "", nil, err
	}

	lost, err := synthParseLost(verdict)
	if err != nil {
		return "", nil, domain.WrapError(err, slog.String("project", key))
	}
	return text, lost, nil
}

func synthReadEntries(memoryDir string) (string, error) {
	entries, err := os.ReadDir(memoryDir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", domain.Error(err, "read memory directory failed", slog.String("memory_dir", memoryDir))
	}

	var out strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == synthIndexFile || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(memoryDir, entry.Name()))
		if err != nil {
			return "", domain.Error(err, "read memory entry failed", slog.String("file", entry.Name()))
		}
		out.WriteString("--- " + entry.Name() + " ---\n" + strings.TrimRight(string(content), "\n") + "\n")
	}
	return out.String(), nil
}

func synthParseLost(answer string) ([]string, error) {
	body := strings.TrimSpace(answer)
	if strings.HasPrefix(body, synthFenceOpen) && strings.HasSuffix(body, synthFenceClose) {
		body = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(body, synthFenceOpen), synthFenceClose))
	}

	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var verdict synthFidelity
	if err := dec.Decode(&verdict); err != nil {
		return nil, domain.Error(errSynthJudgeFormat, "parse fidelity verdict failed", slog.String("cause", err.Error()))
	}
	if dec.More() || verdict.Lost == nil {
		return nil, domain.Error(errSynthJudgeFormat, "fidelity verdict has trailing data or no lost list")
	}
	return verdict.Lost, nil
}

func (r *Runner) synthDistillScan(ctx context.Context, x *run) (string, error) {
	projects, err := corpus.ListProjects(ctx, r.cfg.WorkDir)
	if err != nil {
		return "", err
	}

	var input strings.Builder
	for _, p := range projects {
		index, err := os.ReadFile(filepath.Join(r.cfg.WorkDir, "projects", p.Key, "memory", synthIndexFile))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", domain.Error(err, "read project index failed", slog.String("project", p.Key))
		}
		input.WriteString("=== project: " + p.Key + " ===\n" + string(index) + "\n")
	}

	return x.ask(ctx, synthScanPrompt, ParamSynthModel, input.String())
}

func synthClusterLines(scanOut string) []string {
	raw := strings.TrimSpace(scanOut)
	if raw == "" || raw == synthNothing {
		return nil
	}

	lines := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !json.Valid([]byte(line)) {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func (r *Runner) synthDistillCluster(ctx context.Context, x *run, line string) error {
	var cluster synthCluster
	if err := json.Unmarshal([]byte(line), &cluster); err != nil {
		return x.step(ctx, "distill:unparseable", func(context.Context) (string, error) {
			return "", domain.Error(err, "decode distill cluster failed")
		})
	}

	return x.step(ctx, "distill:"+cluster.Title, func(ctx context.Context) (string, error) {
		if cluster.Title == "" || len(cluster.Entries) == 0 {
			return "", domain.Error(errSynthClusterInvalid, "invalid distill cluster")
		}
		target, _, _ := strings.Cut(cluster.Entries[0], "/")

		entries, err := r.synthClusterEntries(cluster)
		if err != nil {
			return "", err
		}
		text, err := x.ask(ctx, synthWritePrompt, ParamSynthModel, "CLUSTER:\n"+line+"\n\nENTRIES:\n"+entries)
		if err != nil {
			return "", err
		}

		submitter := proposalapply.NewSubmitter(r.deps.Pipeline, synthFlowName, x.id)
		memoryDir := filepath.Join(r.cfg.WorkDir, "projects", target, "memory")
		result, err := ops.Apply(ctx, ops.ApplyOptions{MemoryDir: memoryDir, AllowDelete: true, Submitter: submitter}, strings.NewReader(text))
		if err != nil {
			return "", err
		}

		_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
			WorkDir:  r.cfg.WorkDir,
			Lock:     r.cfg.Lock,
			Message:  "distill: " + cluster.Title,
			Pathspec: "projects/" + target + "/memory",
		})
		if err != nil {
			return "", err
		}
		x.count("proposals", len(result.ProposalIDs))
		out, err := json.Marshal(result)
		if err != nil {
			return "", domain.Error(err, "encode ops result failed")
		}
		return string(out), nil
	})
}

func (r *Runner) synthClusterEntries(cluster synthCluster) (string, error) {
	var out strings.Builder
	for _, entry := range cluster.Entries {
		proj, file, _ := strings.Cut(entry, "/")
		if proj == "" {
			continue
		}
		var content bytes.Buffer
		if err := corpus.ReadEntry(&content, r.cfg.WorkDir, proj, file); err != nil {
			return "", err
		}
		out.WriteString("--- " + proj + "/" + file + " ---\n" + content.String() + "\n")
	}
	return out.String(), nil
}

func (r *Runner) synthHotcacheKey(ctx context.Context, x *run, key, maxBytesRaw string) error {
	return x.step(ctx, "hotcache:"+key, func(ctx context.Context) (string, error) {
		maxBytes, err := strconv.Atoi(maxBytesRaw)
		if err != nil {
			return "", domain.Error(domain.ErrValidation, "hotcache_max_bytes is not an integer", slog.String("value", maxBytesRaw))
		}

		var input bytes.Buffer
		if err := corpus.WriteContext(ctx, &input, r.cfg.ReposDir, r.cfg.WorkDir, key, intake.ScanResult{}); err != nil {
			return "", err
		}
		digest, err := x.ask(ctx, synthDigestPrompt, ParamSynthModel, input.String())
		if err != nil {
			return "", err
		}

		memoryDir := filepath.Join(r.cfg.WorkDir, "projects", key, "memory")
		result, err := hotcache.Write(memoryDir, maxBytes, strings.NewReader(digest))
		if err != nil {
			return "", err
		}

		_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
			WorkDir:  r.cfg.WorkDir,
			Lock:     r.cfg.Lock,
			Message:  "hotcache: " + key,
			Pathspec: "projects/" + key,
		})
		if err != nil {
			return "", err
		}
		out, err := json.Marshal(result)
		if err != nil {
			return "", domain.Error(err, "encode hotcache result failed")
		}
		return string(out), nil
	})
}
