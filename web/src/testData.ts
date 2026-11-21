import type {
  Flow,
  Param,
  Prompt,
  Proposal,
  ProposalDetail,
  Run,
  Step,
} from "./types";

export const makeProposal = (overrides: Partial<Proposal> = {}): Proposal => ({
  id: "p1",
  source_flow: "taskmine",
  source_run: "r1",
  kind: "skill",
  name: "Deploy skill",
  status: "pending",
  created_at: "2026-11-01T10:00:00Z",
  updated_at: "2026-11-01T10:00:00Z",
  decided_at: null,
  decided_by: null,
  claimed_by: null,
  claimed_until: null,
  applied_sha: null,
  error: null,
  files: [{ target_path: "global/skills/deploy.md", content: "# Deploy steps" }],
  ...overrides,
});

export const makeDetail = (
  overrides: Partial<Proposal> = {},
): ProposalDetail => ({
  proposal: makeProposal(overrides),
  events: [
    {
      id: 1,
      proposal_id: overrides.id ?? "p1",
      from_status: null,
      to_status: "pending",
      actor: "taskmine",
      note: null,
      at: "2026-11-01T10:00:00Z",
    },
  ],
});

export const makeRun = (overrides: Partial<Run> = {}): Run => ({
  id: "run-1",
  flow: "intake",
  trigger: "schedule",
  status: "succeeded",
  started_at: "2026-11-02T09:00:00Z",
  finished_at: "2026-11-02T09:01:00Z",
  error: null,
  summary: { clients: 3, note: "ok" },
  ...overrides,
});

export const makeStep = (overrides: Partial<Step> = {}): Step => ({
  id: 1,
  run_id: "run-1",
  step: "scan",
  status: "succeeded",
  started_at: "2026-11-02T09:00:00Z",
  finished_at: "2026-11-02T09:00:30Z",
  error: null,
  output_tail: null,
  ...overrides,
});

export const makeFlow = (overrides: Partial<Flow> = {}): Flow => ({
  name: "intake",
  schedule: "*/45 * * * *",
  default_schedule: "*/45 * * * *",
  running: false,
  last_run: makeRun(),
  ...overrides,
});

export const makePrompt = (overrides: Partial<Prompt> = {}): Prompt => ({
  name: "intake-judge",
  content: "default text",
  default: "default text",
  overridden: false,
  updated_at: null,
  ...overrides,
});

export const makeParam = (overrides: Partial<Param> = {}): Param => ({
  key: "synth_model",
  value: "sonnet",
  default: "sonnet",
  kind: "model",
  description: "Model for synthesis",
  overridden: false,
  ...overrides,
});
