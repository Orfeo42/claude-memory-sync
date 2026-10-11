package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/gitutil"
	"claude-memory-sync/internal/guardrails"
	"claude-memory-sync/internal/pipeline"
	"claude-memory-sync/internal/pipeline/client"
)

const pipelineHTTPTimeout = 30 * time.Second

const errProposalsFailed cliError = "some proposals failed to apply"

type fileGroup struct {
	Name  string
	Files []pipeline.File
}

type submitOutcome struct {
	Created  []string     `json:"created"`
	Rejected []rejectedOp `json:"rejected"`
}

type proposalSubmitter struct {
	client     *client.Client
	sourceFlow string
	sourceRun  string
}

type appliedProposal struct {
	ID      string        `json:"id"`
	Kind    pipeline.Kind `json:"kind"`
	Name    string        `json:"name"`
	Targets []string      `json:"targets"`
}

type applyApprovedResult struct {
	Applied          []string          `json:"applied"`
	Failed           []string          `json:"failed"`
	AppliedProposals []appliedProposal `json:"applied_proposals"`
}

type importRejectionsResult struct {
	Imported int `json:"imported"`
}

const importedRejectionReason = "imported from taskmine-rejected"

type expireResult struct {
	Expired int `json:"expired"`
}

type reviseInput struct {
	Files []pipeline.File `json:"files"`
}

func newPipelineClient() (*client.Client, error) {
	cfg, err := client.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	return client.New(cfg, &http.Client{Timeout: pipelineHTTPTimeout}), nil
}

func proposalSubcommands() map[string]subcommandFunc {
	return map[string]subcommandFunc{
		"list":              cmdProposalList,
		"get":               cmdProposalGet,
		"rejections":        cmdProposalRejections,
		"decide":            cmdProposalDecide,
		"revise":            cmdProposalRevise,
		"retry":             cmdProposalRetry,
		"apply-approved":    cmdProposalApplyApproved,
		"expire":            cmdProposalExpire,
		"import":            cmdProposalImport,
		"import-rejections": cmdProposalImportRejections,
	}
}

func cmdProposal(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return domain.Error(errUnknownSubcommand, "proposal subcommand required")
	}

	handler, ok := proposalSubcommands()[args[0]]
	if !ok {
		return domain.Error(errUnknownSubcommand, "unrecognized proposal subcommand")
	}

	return handler(ctx, args[1:])
}

func cmdProposalList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal list", flag.ExitOnError)
	statusFlag := fs.String("status", "", "status filter")
	kindFlag := fs.String("kind", "", "kind filter")
	limit := fs.Int("limit", 0, "max results")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal list flags failed")
	}

	params := client.ListParams{Limit: *limit}
	if *statusFlag != "" {
		status, err := pipeline.ParseStatus(*statusFlag)
		if err != nil {
			return err
		}
		params.Status = &status
	}
	if *kindFlag != "" {
		kind, err := pipeline.ParseKind(*kindFlag)
		if err != nil {
			return err
		}
		params.Kind = &kind
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposals, err := pipelineClient.List(ctx, params)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposals); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalGet(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal get", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal get flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	detail, err := pipelineClient.Get(ctx, *id)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(detail); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRejections(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal rejections", flag.ExitOnError)
	kindFlag := fs.String("kind", "", "kind filter")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal rejections flags failed")
	}

	var kind *pipeline.Kind
	if *kindFlag != "" {
		parsed, err := pipeline.ParseKind(*kindFlag)
		if err != nil {
			return err
		}
		kind = &parsed
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	rejections, err := pipelineClient.Rejections(ctx, kind)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(rejections); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalDecide(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal decide", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	decision := fs.String("decision", "", "approve or reject")
	actor := fs.String("actor", "", "actor")
	reason := fs.String("reason", "", "reason")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal decide flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Decide(ctx, *id, client.DecideRequest{Decision: *decision, Actor: *actor, Reason: *reason})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRevise(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal revise", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal revise flags failed")
	}

	var input reviseInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return domain.Error(domain.ErrValidation, "decode revise input failed", slog.String("cause", err.Error()))
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Revise(ctx, *id, client.ReviseRequest{Actor: *actor, Files: input.Files})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalRetry(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal retry", flag.ExitOnError)
	id := fs.String("id", "", "proposal id")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal retry flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	proposal, err := pipelineClient.Retry(ctx, *id, *actor)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(proposal); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalExpire(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal expire", flag.ExitOnError)
	olderThan := fs.Duration("older-than", 0, "expire pending proposals older than this duration")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal expire flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}
	expired, err := pipelineClient.Expire(ctx, *actor, int(olderThan.Hours()))
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(expireResult{Expired: expired}); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func cmdProposalApplyApproved(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal apply-approved", flag.ExitOnError)
	workDir := fs.String("work-dir", "", "work directory")
	lock := fs.String("lock", "", "lock file path")
	worker := fs.String("worker", "", "worker name")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal apply-approved flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	result, applyErr := applyApproved(ctx, pipelineClient, *workDir, *lock, *worker)
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return applyErr
}

type committedProposal struct {
	ID      string
	SHA     string
	Kind    pipeline.Kind
	Name    string
	Targets []string
}

func applyApproved(ctx context.Context, pipelineClient *client.Client, workDir, lock, worker string) (applyApprovedResult, error) {
	result := applyApprovedResult{Applied: []string{}, Failed: []string{}, AppliedProposals: []appliedProposal{}}

	committed, err := claimAndCommit(ctx, pipelineClient, workDir, lock, worker, &result)
	if err != nil {
		return result, err
	}

	if err := finalizeCommitted(ctx, pipelineClient, workDir, lock, worker, committed, &result); err != nil {
		return result, err
	}

	if len(result.Failed) > 0 {
		return result, domain.Error(errProposalsFailed, "apply-approved finished with failures", slog.Int("failed", len(result.Failed)))
	}
	return result, nil
}

func claimAndCommit(ctx context.Context, pipelineClient *client.Client, workDir, lock, worker string, result *applyApprovedResult) ([]committedProposal, error) {
	committed := make([]committedProposal, 0)

	for {
		proposal, err := pipelineClient.Claim(ctx, worker, 0)
		if errors.Is(err, pipeline.ErrNothingToClaim) {
			return committed, nil
		}
		if err != nil {
			return committed, err
		}

		sha, applyErr := applyClaimed(ctx, workDir, lock, proposal)
		if applyErr == nil {
			targets := make([]string, 0, len(proposal.Files))
			for _, f := range proposal.Files {
				targets = append(targets, f.TargetPath)
			}
			committed = append(committed, committedProposal{ID: proposal.ID, SHA: sha, Kind: proposal.Kind, Name: proposal.Name, Targets: targets})
			continue
		}

		if _, err := pipelineClient.MarkFailed(ctx, proposal.ID, worker, applyErr.Error()); err != nil {
			return committed, err
		}
		result.Failed = append(result.Failed, proposal.ID)
	}
}

func finalizeCommitted(ctx context.Context, pipelineClient *client.Client, workDir, lock, worker string, committed []committedProposal, result *applyApprovedResult) error {
	if len(committed) == 0 {
		return nil
	}

	if pushErr := pushWithLock(ctx, workDir, lock); pushErr != nil {
		for _, c := range committed {
			if _, err := pipelineClient.MarkFailed(ctx, c.ID, worker, "push failed: "+pushErr.Error()); err != nil {
				return err
			}
			result.Failed = append(result.Failed, c.ID)
		}
		return pushErr
	}

	for _, c := range committed {
		if _, err := pipelineClient.MarkApplied(ctx, c.ID, worker, c.SHA); err != nil {
			return err
		}
		result.Applied = append(result.Applied, c.ID)
		result.AppliedProposals = append(result.AppliedProposals, appliedProposal{ID: c.ID, Kind: c.Kind, Name: c.Name, Targets: c.Targets})
	}
	return nil
}

func resolveProposalDests(workDir string, proposal pipeline.Proposal) ([]string, error) {
	if len(proposal.Files) == 0 {
		return nil, domain.Error(domain.ErrValidation, "proposal has no files")
	}

	dests := make([]string, 0, len(proposal.Files))
	for _, f := range proposal.Files {
		if err := pipeline.ValidateTarget(f.TargetPath); err != nil {
			return nil, domain.Error(err, "reject proposal target")
		}
		if err := guardrails.ScanSecrets([]byte(f.Content)); err != nil {
			return nil, domain.Error(err, "proposal secret scan failed")
		}
		dest, err := resolveProposalDest(workDir, f.TargetPath)
		if err != nil {
			return nil, err
		}
		dests = append(dests, dest)
	}
	return dests, nil
}

func applyClaimed(ctx context.Context, workDir, lock string, proposal pipeline.Proposal) (string, error) {
	dests, err := resolveProposalDests(workDir, proposal)
	if err != nil {
		return "", err
	}

	message := "proposal: apply " + string(proposal.Kind) + " " + proposal.Name
	pathspec := pipeline.PathspecOf(proposal.Files[0].TargetPath)
	sha := ""

	err = gitutil.WithFileLock(lock, func() error {
		for i, f := range proposal.Files {
			if err := os.MkdirAll(filepath.Dir(dests[i]), 0o750); err != nil {
				return domain.Error(err, "create proposal target directory failed")
			}
			if err := writeProposalContent(dests[i], []byte(f.Content)); err != nil {
				return domain.Error(err, "write proposal file failed")
			}
		}

		if _, err := commitPathspec(ctx, workDir, message, pathspec); err != nil {
			return err
		}

		head, err := gitutil.RunOutput(ctx, workDir, "rev-parse", "HEAD")
		if err != nil {
			return domain.Error(err, "read HEAD sha failed")
		}
		sha = head
		return nil
	})
	if err != nil {
		return "", err
	}
	return sha, nil
}

func cmdProposalImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal import", flag.ExitOnError)
	dir := fs.String("dir", "", "staging directory")
	sourceFlow := fs.String("source-flow", "", "source flow")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal import flags failed")
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	outcome, err := importProposals(ctx, pipelineClient, *dir, *sourceFlow)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(outcome); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func importProposals(ctx context.Context, pipelineClient *client.Client, dir, sourceFlow string) (submitOutcome, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return submitOutcome{}, domain.Error(err, "read staging directory failed")
	}

	files := make([]pipeline.File, 0, len(entries))
	rejected := make([]rejectedOp, 0)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".path") {
			continue
		}

		flattened := filepath.Join(dir, entry.Name())
		target, err := resolveProposalRealPath(flattened)
		if err != nil {
			return submitOutcome{}, err
		}
		if err := pipeline.ValidateTarget(target); err != nil {
			rejected = append(rejected, rejectedOp{Name: entry.Name(), Reason: reasonInvalidProposalPath})
			continue
		}

		content, err := os.ReadFile(flattened)
		if err != nil {
			return submitOutcome{}, domain.Error(err, "read staged proposal file failed")
		}
		files = append(files, pipeline.File{TargetPath: target, Content: string(content)})
	}

	submitter := proposalSubmitter{client: pipelineClient, sourceFlow: sourceFlow, sourceRun: filepath.Base(filepath.Clean(dir))}
	outcome, err := submitter.submit(ctx, groupFiles(files))
	if err != nil {
		return submitOutcome{}, err
	}

	outcome.Rejected = append(rejected, outcome.Rejected...)
	return outcome, nil
}

func groupFiles(files []pipeline.File) []fileGroup {
	groups := make([]fileGroup, 0, len(files))
	index := make(map[string]int, len(files))
	for _, f := range files {
		name := pipeline.GroupOf(f.TargetPath)
		i, ok := index[name]
		if !ok {
			i = len(groups)
			index[name] = i
			groups = append(groups, fileGroup{Name: name})
		}
		groups[i].Files = append(groups[i].Files, f)
	}
	return groups
}

func (s *proposalSubmitter) submit(ctx context.Context, groups []fileGroup) (submitOutcome, error) {
	outcome := submitOutcome{Created: []string{}, Rejected: []rejectedOp{}}

	for _, group := range groups {
		proposal, err := s.client.Create(ctx, client.CreateRequest{SourceFlow: s.sourceFlow, SourceRun: s.sourceRun, Files: group.Files})
		if err == nil {
			outcome.Created = append(outcome.Created, proposal.ID)
			continue
		}

		var upstream *domain.UpstreamError
		isRejection := errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrValidation)
		if !isRejection || !errors.As(err, &upstream) {
			return submitOutcome{}, err
		}
		outcome.Rejected = append(outcome.Rejected, rejectedOp{Name: group.Name, Reason: upstream.Body})
	}

	return outcome, nil
}

func cmdProposalImportRejections(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("proposal import-rejections", flag.ExitOnError)
	file := fs.String("file", "", "legacy rejected-names file")
	actor := fs.String("actor", "", "actor")
	if err := fs.Parse(args); err != nil {
		return domain.Error(err, "parse proposal import-rejections flags failed")
	}
	if *file == "" {
		return domain.Error(domain.ErrValidation, "file flag required")
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		return domain.Error(domain.ErrValidation, "read rejected-names file failed", slog.String("cause", err.Error()))
	}

	pipelineClient, err := newPipelineClient()
	if err != nil {
		return err
	}

	reason := importedRejectionReason
	imported := 0
	for _, name := range legacyRejectionNames(string(data)) {
		kind := pipeline.KindSkill
		if strings.HasSuffix(name, ".md") {
			kind = pipeline.KindOf(name)
		}
		if err := pipelineClient.AddRejection(ctx, client.AddRejectionRequest{Kind: kind, Name: name, Actor: *actor, Reason: &reason}); err != nil {
			return err
		}
		imported++
	}

	if err := json.NewEncoder(os.Stdout).Encode(importRejectionsResult{Imported: imported}); err != nil {
		return domain.Error(err, "encode result failed")
	}
	return nil
}

func legacyRejectionNames(data string) []string {
	seen := map[string]bool{}
	names := make([]string, 0)
	for _, line := range strings.Split(data, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
