package flows

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"claude-memory-sync/internal/corpus"
	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/intake"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/workrepo"
)

const intakeJudgePrompt = "intake-judge"

func (r *Runner) intake(ctx context.Context, x *run) error {
	err := x.step(ctx, "ensure_work", func(ctx context.Context) (string, error) {
		return "", gitutil.WithFileLock(r.cfg.Lock, func() error {
			return workrepo.EnsureWork(ctx, r.cfg.ReposDir, r.cfg.WorkDir)
		})
	})
	if err != nil {
		return err
	}

	var scan intake.ScanResult
	err = x.step(ctx, "scan", func(ctx context.Context) (string, error) {
		var err error
		scan, err = intake.Scan(ctx, r.cfg.ReposDir)
		if err != nil {
			return "", err
		}
		out, err := json.Marshal(scan)
		if err != nil {
			return "", domain.Error(err, "encode step result failed")
		}
		return string(out), nil
	})
	if err != nil {
		return err
	}

	err = x.step(ctx, "config_merge", func(ctx context.Context) (string, error) {
		return r.intakeConfigMerge(ctx, scan)
	})
	if err != nil {
		return err
	}

	keys := intakeProjectKeys(scan)
	err = x.step(ctx, "project_keys", func(_ context.Context) (string, error) {
		out, err := json.Marshal(keys)
		if err != nil {
			return "", domain.Error(err, "encode step result failed")
		}
		return string(out), nil
	})
	if err != nil {
		return err
	}

	if err := r.intakeJudgeAll(ctx, x, keys, scan); err != nil {
		return err
	}

	err = x.step(ctx, "push", func(ctx context.Context) (string, error) {
		return "", workrepo.Push(ctx, r.cfg.WorkDir, r.cfg.Lock)
	})
	if err != nil {
		return err
	}

	return x.step(ctx, "advance_markers", func(ctx context.Context) (string, error) {
		return "", r.intakeAdvanceMarkers(ctx, scan)
	})
}

func (r *Runner) intakeJudgeAll(ctx context.Context, x *run, keys []string, scan intake.ScanResult) error {
	x.count("projects", len(keys))
	for _, key := range keys {
		err := x.step(ctx, "judge:"+key, func(ctx context.Context) (string, error) {
			applied, err := r.intakeJudge(ctx, x, key, scan)
			if err != nil {
				return "", err
			}
			x.count("applied", len(applied.Applied))
			x.count("rejected", len(applied.Rejected))
			out, err := json.Marshal(applied)
			if err != nil {
				return "", domain.Error(err, "encode step result failed")
			}
			return string(out), nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) intakeConfigMerge(ctx context.Context, scan intake.ScanResult) (string, error) {
	merged, err := intake.MergeConfig(ctx, r.cfg.ReposDir, r.cfg.WorkDir, scan)
	if err != nil {
		return "", err
	}
	_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
		WorkDir:  r.cfg.WorkDir,
		Lock:     r.cfg.Lock,
		Message:  "intake: config",
		Pathspec: "global",
	})
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return "", domain.Error(err, "encode step result failed")
	}
	return string(out), nil
}

func (r *Runner) intakeAdvanceMarkers(ctx context.Context, scan intake.ScanResult) error {
	for _, client := range scan.Clients {
		if client.ID == "" {
			continue
		}
		if err := intake.AdvanceMarker(ctx, r.cfg.ReposDir, client.ID, client.Head); err != nil {
			return domain.WrapError(err, slog.String("client", client.ID))
		}
	}
	return nil
}

func (r *Runner) intakeJudge(ctx context.Context, x *run, key string, scan intake.ScanResult) (ops.ApplyResult, error) {
	var input bytes.Buffer
	if err := corpus.WriteContext(ctx, &input, r.cfg.ReposDir, r.cfg.WorkDir, key, scan); err != nil {
		return ops.ApplyResult{}, domain.WrapError(err, slog.String("project", key))
	}

	output, err := x.ask(ctx, intakeJudgePrompt, ParamSynthModel, input.String())
	if err != nil {
		return ops.ApplyResult{}, domain.WrapError(err, slog.String("project", key))
	}

	opts := ops.ApplyOptions{MemoryDir: filepath.Join(r.cfg.WorkDir, "projects", key, "memory")}
	applied, err := ops.Apply(ctx, opts, strings.NewReader(output))
	if err != nil {
		return ops.ApplyResult{}, domain.WrapError(err, slog.String("project", key))
	}

	_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
		WorkDir:  r.cfg.WorkDir,
		Lock:     r.cfg.Lock,
		Message:  "intake: " + key,
		Pathspec: "projects/" + key + "/memory",
	})
	if err != nil {
		return ops.ApplyResult{}, domain.WrapError(err, slog.String("project", key))
	}
	return applied, nil
}

func intakeProjectKeys(scan intake.ScanResult) []string {
	seen := map[string]bool{}
	keys := []string{}
	for _, client := range scan.Clients {
		for _, cand := range client.Memory {
			if seen[cand.Project] {
				continue
			}
			seen[cand.Project] = true
			keys = append(keys, cand.Project)
		}
	}
	slices.Sort(keys)
	return keys
}
