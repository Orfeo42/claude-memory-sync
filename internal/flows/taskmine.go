package flows

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"claude-memory-sync/internal/corpus"
	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/ops"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
	"claude-memory-sync/internal/proposalapply"
	"claude-memory-sync/internal/workrepo"
)

func (r *Runner) taskmine(ctx context.Context, x *run) error {
	err := x.step(ctx, "ensure_work", func(ctx context.Context) (string, error) {
		return "", gitutil.WithFileLock(r.cfg.Lock, func() error {
			return workrepo.EnsureWork(ctx, r.cfg.ReposDir, r.cfg.WorkDir)
		})
	})
	if err != nil {
		return err
	}

	return x.step(ctx, "draft", func(ctx context.Context) (string, error) {
		return r.taskmineDraft(ctx, x)
	})
}

func (r *Runner) taskmineDraft(ctx context.Context, x *run) (string, error) {
	skills, err := r.taskmineSkillInventory()
	if err != nil {
		return "", err
	}
	excluded, err := r.taskmineExcluded(ctx, x)
	if err != nil {
		return "", err
	}
	indexes, err := r.taskmineProjectIndexes(ctx)
	if err != nil {
		return "", err
	}

	output, err := x.ask(ctx, "taskmine-draft", ParamSynthModel, skills+excluded+indexes)
	if err != nil {
		return "", domain.WrapError(err)
	}

	submitter := proposalapply.NewSubmitter(r.deps.Pipeline, "taskmine", x.id)
	result, err := ops.Apply(ctx, ops.ApplyOptions{Submitter: submitter}, strings.NewReader(output))
	if err != nil {
		return "", domain.WrapError(err)
	}
	x.count("proposals", len(result.ProposalIDs))

	encoded, err := json.Marshal(result)
	if err != nil {
		return "", domain.Error(err, "encode taskmine result failed")
	}
	return string(encoded), nil
}

func (r *Runner) taskmineSkillInventory() (string, error) {
	var section strings.Builder
	section.WriteString("=== existing skills — patterns already covered, do NOT re-propose ===\n")

	skills, err := filepath.Glob(filepath.Join(r.cfg.WorkDir, "global", "skills", "*", "SKILL.md"))
	if err != nil {
		return "", domain.Error(err, "glob skills failed")
	}
	for _, skill := range skills {
		data, err := os.ReadFile(skill)
		if err != nil {
			return "", domain.Error(err, "read skill failed", slog.String("path", skill))
		}
		description := ""
		for line := range strings.SplitSeq(string(data), "\n") {
			if value, ok := strings.CutPrefix(line, "description:"); ok {
				description = strings.TrimLeft(value, " \t")
				break
			}
		}
		section.WriteString("- " + filepath.Base(filepath.Dir(skill)) + ": " + description + "\n")
	}
	section.WriteString("\n")
	return section.String(), nil
}

func (r *Runner) taskmineExcluded(ctx context.Context, x *run) (string, error) {
	rejections, err := r.deps.Pipeline.Rejections(ctx, nil)
	if err != nil {
		return "", domain.WrapError(err)
	}
	rejected := map[string]bool{}
	for _, rejection := range rejections {
		rejected[rejection.Name] = true
	}
	x.count("rejected", len(rejected))

	pending := map[string]bool{}
	for _, status := range []pipeline.Status{pipeline.StatusPending, pipeline.StatusApproved} {
		proposals, err := r.deps.Pipeline.List(ctx, client.ListParams{Status: &status})
		if err != nil {
			return "", domain.WrapError(err, slog.String("status", string(status)))
		}
		for _, proposal := range proposals {
			pending[proposal.Name] = true
		}
	}

	return taskmineNameSection("=== previously rejected proposals — do NOT re-propose ===", rejected) +
		taskmineNameSection("=== proposals currently pending human review — do NOT re-propose ===", pending), nil
}

func (r *Runner) taskmineProjectIndexes(ctx context.Context) (string, error) {
	projects, err := corpus.ListProjects(ctx, r.cfg.WorkDir)
	if err != nil {
		return "", domain.WrapError(err)
	}

	var section strings.Builder
	for _, project := range projects {
		index, err := os.ReadFile(filepath.Join(r.cfg.WorkDir, "projects", project.Key, "memory", "MEMORY.md"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", domain.Error(err, "read memory index failed", slog.String("project", project.Key))
		}
		section.WriteString("=== project: " + project.Key + " ===\n" + string(index) + "\n")
	}
	return section.String(), nil
}

func taskmineNameSection(header string, names map[string]bool) string {
	if len(names) == 0 {
		return ""
	}
	return header + "\n" + strings.Join(slices.Sorted(maps.Keys(names)), "\n") + "\n\n"
}
