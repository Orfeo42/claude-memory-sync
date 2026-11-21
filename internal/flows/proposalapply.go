package flows

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/proposalapply"
	"claude-memory-sync/internal/workrepo"
)

type applyError string

func (e applyError) Error() string { return string(e) }

const errApplyReportFailed = applyError("proposal apply finished with failures")

const (
	applyActor         = "proposal-apply"
	applyExpireAfter   = 336 * time.Hour
	applyCleanupPrompt = "taskmine-cleanup"
)

var (
	applyEvidenceHeading = regexp.MustCompile(`^#+\s*Evidence`)
	applyAnyHeading      = regexp.MustCompile(`^#+\s`)
	applyEvidenceRef     = regexp.MustCompile(`[a-z0-9][a-z0-9-]*/[A-Za-z0-9._-]+\.md`)
)

type applyOutput struct {
	proposalapply.ApplyApprovedResult
	ExitCode int     `json:"exit_code"`
	Error    *string `json:"error,omitempty"`
}

func (r *Runner) proposalApply(ctx context.Context, x *run) error {
	if err := r.applyPrepare(ctx, x); err != nil {
		return err
	}

	var applied applyOutput
	err := x.step(ctx, "apply", func(ctx context.Context) (string, error) {
		result, applyErr := proposalapply.ApplyApproved(ctx, r.deps.Pipeline, proposalapply.ApplyApprovedOptions{
			WorkDir: r.cfg.WorkDir,
			Lock:    r.cfg.Lock,
			Worker:  r.cfg.Worker + ":" + x.id,
		})
		applied = applyOutput{ApplyApprovedResult: result}
		if applyErr != nil {
			applied.ExitCode = 1
			msg := domain.UserMessage(applyErr)
			applied.Error = &msg
		}
		out, err := json.Marshal(applied)
		if err != nil {
			return "", domain.Error(err, "encode apply result failed")
		}
		return string(out), nil
	})
	if err != nil {
		return err
	}

	err = x.step(ctx, "cleanup", func(ctx context.Context) (string, error) {
		return r.applyCleanup(ctx, x, applied.AppliedProposals)
	})
	if err != nil {
		return err
	}

	err = x.step(ctx, "report", func(context.Context) (string, error) {
		x.count("applied", len(applied.Applied))
		x.count("failed", len(applied.Failed))
		report := "applied=" + strconv.Itoa(len(applied.Applied)) + " failed=" + strconv.Itoa(len(applied.Failed))
		if len(applied.Failed) > 0 || applied.ExitCode != 0 {
			return report, domain.Error(errApplyReportFailed, "apply reported failures",
				slog.Int("failed", len(applied.Failed)), slog.Int("exit_code", applied.ExitCode))
		}
		return report, nil
	})
	if errors.Is(err, errApplyReportFailed) {
		return nil
	}
	return err
}

func (r *Runner) applyPrepare(ctx context.Context, x *run) error {
	err := x.step(ctx, "ensure_work", func(ctx context.Context) (string, error) {
		return "", gitutil.WithFileLock(r.cfg.Lock, func() error {
			return workrepo.EnsureWork(ctx, r.cfg.ReposDir, r.cfg.WorkDir)
		})
	})
	if err != nil {
		return err
	}

	return x.step(ctx, "expire", func(ctx context.Context) (string, error) {
		expired, err := proposalapply.Expire(ctx, r.deps.Pipeline, applyActor, applyExpireAfter)
		if err != nil {
			return "", err
		}
		out, err := json.Marshal(expired)
		if err != nil {
			return "", domain.Error(err, "encode expire result failed")
		}
		return string(out), nil
	})
}

func (r *Runner) applyCleanup(ctx context.Context, x *run, proposals []proposalapply.AppliedProposal) (string, error) {
	cleaned := 0
	for _, p := range proposals {
		if p.Kind != pipeline.KindSkill {
			continue
		}
		for _, target := range p.Targets {
			n, err := r.applyCleanupSkill(ctx, x, target)
			if err != nil {
				return "", err
			}
			cleaned += n
		}
	}
	if cleaned == 0 {
		return "cleaned=0", nil
	}
	if err := workrepo.Push(ctx, r.cfg.WorkDir, r.cfg.Lock); err != nil {
		return "", err
	}
	return "cleaned=1", nil
}

func (r *Runner) applyCleanupSkill(ctx context.Context, x *run, target string) (int, error) {
	if ok, _ := path.Match("global/skills/*/SKILL.md", target); !ok {
		return 0, nil
	}
	skill, err := os.ReadFile(filepath.Join(r.cfg.WorkDir, filepath.FromSlash(target)))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, domain.Error(err, "read promoted skill failed", slog.String("target", target))
	}
	group := strings.Split(target, "/")[2]

	refs := applyEvidenceRefs(string(skill))
	keys := make([]string, 0, len(refs))
	byKey := map[string][]string{}
	for _, ref := range refs {
		key, entry, _ := strings.Cut(ref, "/")
		if _, seen := byKey[key]; !seen {
			keys = append(keys, key)
		}
		byKey[key] = append(byKey[key], entry)
	}

	sort.Strings(keys)
	cleaned := 0
	for _, key := range keys {
		done, err := r.applyCleanupProject(ctx, x, group, string(skill), key, byKey[key])
		if err != nil {
			return 0, err
		}
		if done {
			cleaned = 1
		}
	}
	return cleaned, nil
}

func (r *Runner) applyCleanupProject(ctx context.Context, x *run, group, skill, key string, entries []string) (bool, error) {
	memoryDir := filepath.Join(r.cfg.WorkDir, "projects", key, "memory")
	info, err := os.Stat(memoryDir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, domain.Error(err, "stat memory dir failed", slog.String("memory_dir", memoryDir))
	}
	if !info.IsDir() {
		return false, nil
	}

	input, found, err := applyCleanupInput(memoryDir, group, skill, key, entries)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	out, err := x.ask(ctx, applyCleanupPrompt, ParamSynthModel, input)
	if err != nil {
		return false, err
	}
	if _, err := ops.Apply(ctx, ops.ApplyOptions{MemoryDir: memoryDir, AllowDelete: true}, strings.NewReader(out)); err != nil {
		return false, err
	}
	_, err = workrepo.Commit(ctx, workrepo.CommitOptions{
		WorkDir:  r.cfg.WorkDir,
		Lock:     r.cfg.Lock,
		Message:  "taskmine: cleanup " + key + " after " + group,
		Pathspec: "projects/" + key,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func applyCleanupInput(memoryDir, group, skill, key string, entries []string) (string, bool, error) {
	var input strings.Builder
	input.WriteString("=== new skill: " + group + " ===\n")
	input.WriteString(skill)
	input.WriteString("\n=== candidate entries (project: " + key + ") ===\n")
	found := false
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(memoryDir, entry))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", false, domain.Error(err, "read memory entry failed", slog.String("entry", entry))
		}
		found = true
		input.WriteString("===ENTRY: " + entry + "===\n")
		input.Write(content)
		input.WriteString("\n")
	}
	return input.String(), found, nil
}

func applyEvidenceRefs(skill string) []string {
	var section strings.Builder
	inEvidence := false
	for _, line := range strings.Split(skill, "\n") {
		if applyEvidenceHeading.MatchString(line) {
			inEvidence = true
			continue
		}
		if applyAnyHeading.MatchString(line) {
			inEvidence = false
			continue
		}
		if inEvidence {
			section.WriteString(line + "\n")
		}
	}

	seen := map[string]bool{}
	refs := make([]string, 0)
	for _, ref := range applyEvidenceRef.FindAllString(section.String(), -1) {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}
