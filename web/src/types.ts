export type ProposalStatus =
  | "pending"
  | "approved"
  | "applying"
  | "applied"
  | "rejected"
  | "expired"
  | "failed";

export type ProposalFile = {
  target_path: string;
  content: string;
};

export type Proposal = {
  id: string;
  source_flow: string;
  source_run: string;
  kind: string;
  name: string;
  status: ProposalStatus;
  created_at: string;
  updated_at: string;
  decided_at?: string | null;
  decided_by?: string | null;
  claimed_by?: string | null;
  claimed_until?: string | null;
  applied_sha?: string | null;
  error?: string | null;
  files?: ProposalFile[] | null;
};

export type ProposalEvent = {
  id: number;
  proposal_id: string;
  from_status?: string | null;
  to_status: string;
  actor: string;
  note?: string | null;
  at: string;
};

export type ProposalDetail = {
  proposal: Proposal;
  events: ProposalEvent[];
};

export type DecideResult = {
  proposal: Proposal;
  apply_started: boolean;
};

export type RevisionResult = {
  applied: string[];
  proposal: Proposal;
};

export type ChatRole = "user" | "assistant";

export type ChatMessage = {
  id: number;
  role: ChatRole;
  content: string;
};

export type ChatTurn = {
  role: ChatRole;
  content: string;
};

export const MODELS = ["sonnet", "opus", "haiku"] as const;
export type Model = (typeof MODELS)[number];

export type RunStatus =
  | "running"
  | "succeeded"
  | "failed"
  | "partial"
  | "interrupted";

export type StepStatus = "succeeded" | "failed" | "skipped" | "lossy";

export type Run = {
  id: string;
  flow: string;
  trigger: string;
  status: RunStatus;
  started_at: string;
  finished_at?: string | null;
  error?: string | null;
  summary?: Record<string, unknown> | null;
};

export type Step = {
  id: number;
  run_id: string;
  step: string;
  status: StepStatus;
  started_at: string;
  finished_at: string;
  error?: string | null;
  output_tail?: string | null;
};

export type RunDetail = {
  run: Run;
  steps: Step[];
};

export type Flow = {
  name: string;
  schedule: string;
  default_schedule: string;
  running: boolean;
  last_run?: Run | null;
};

export type PromptSummary = {
  name: string;
  overridden: boolean;
  updated_at?: string | null;
};

export type Prompt = {
  name: string;
  content: string;
  default: string;
  overridden: boolean;
  updated_at?: string | null;
};

export type PromptVersion = {
  content: string | null;
  deleted: boolean;
  created_at: string;
  created_by: string;
};

export type ParamKind = "model" | "bool" | "int";

export type Param = {
  key: string;
  value: string;
  default: string;
  kind: ParamKind;
  description: string;
  overridden: boolean;
};

export type TokenStatus = {
  set: boolean;
  updated_at?: string | null;
};

export type Settings = {
  params: Param[];
  token: TokenStatus;
};

export type FootprintProject = {
  key: string;
  memory_lines: number;
  memory_bytes: number;
  entries: number;
  pct_of_limit: number;
};

export type FootprintGlobal = {
  claude_md_bytes: number;
  rules_unconditional_bytes: number;
  rules_scoped_bytes: number;
  rules_unconditional: string[];
  rules_scoped: string[];
  skills: number;
};

export type FootprintLimits = {
  memory_lines: number;
  memory_bytes: number;
  warn_pct: number;
};

export type Footprint = {
  generated_at: string;
  limits: FootprintLimits;
  projects: FootprintProject[];
  global: FootprintGlobal;
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const isStringOrNull = (value: unknown): value is string | null =>
  value === null || typeof value === "string";

const isOptionalString = (value: unknown): value is string | null | undefined =>
  value === undefined || isStringOrNull(value);

const isStringArray = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === "string");

const isArrayOf = (
  value: unknown,
  guard: (item: unknown) => boolean,
): boolean => Array.isArray(value) && value.every(guard);

export const isProposalStatus = (value: unknown): value is ProposalStatus =>
  value === "pending" ||
  value === "approved" ||
  value === "applying" ||
  value === "applied" ||
  value === "rejected" ||
  value === "expired" ||
  value === "failed";

export const isProposalFile = (value: unknown): value is ProposalFile =>
  isRecord(value) &&
  typeof value.target_path === "string" &&
  typeof value.content === "string";

export const isProposal = (value: unknown): value is Proposal =>
  isRecord(value) &&
  typeof value.id === "string" &&
  typeof value.source_flow === "string" &&
  typeof value.source_run === "string" &&
  typeof value.kind === "string" &&
  typeof value.name === "string" &&
  isProposalStatus(value.status) &&
  typeof value.created_at === "string" &&
  typeof value.updated_at === "string" &&
  isOptionalString(value.decided_at) &&
  isOptionalString(value.decided_by) &&
  isOptionalString(value.claimed_by) &&
  isOptionalString(value.claimed_until) &&
  isOptionalString(value.applied_sha) &&
  isOptionalString(value.error) &&
  (value.files === null ||
    value.files === undefined ||
    isArrayOf(value.files, isProposalFile));

export const isProposalEvent = (value: unknown): value is ProposalEvent =>
  isRecord(value) &&
  typeof value.id === "number" &&
  typeof value.proposal_id === "string" &&
  isOptionalString(value.from_status) &&
  typeof value.to_status === "string" &&
  typeof value.actor === "string" &&
  isOptionalString(value.note) &&
  typeof value.at === "string";

export const isProposalList = (
  value: unknown,
): value is { proposals: Proposal[] } =>
  isRecord(value) && isArrayOf(value.proposals, isProposal);

export const isProposalDetail = (value: unknown): value is ProposalDetail =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  isArrayOf(value.events, isProposalEvent);

export const isDecideResult = (value: unknown): value is DecideResult =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  typeof value.apply_started === "boolean";

export const isChatReply = (value: unknown): value is { reply: string } =>
  isRecord(value) && typeof value.reply === "string";

export const isRevisionResult = (value: unknown): value is RevisionResult =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  isStringArray(value.applied);

const isRunStatus = (value: unknown): value is RunStatus =>
  value === "running" ||
  value === "succeeded" ||
  value === "failed" ||
  value === "partial" ||
  value === "interrupted";

const isStepStatus = (value: unknown): value is StepStatus =>
  value === "succeeded" ||
  value === "failed" ||
  value === "skipped" ||
  value === "lossy";

export const isRun = (value: unknown): value is Run =>
  isRecord(value) &&
  typeof value.id === "string" &&
  typeof value.flow === "string" &&
  typeof value.trigger === "string" &&
  isRunStatus(value.status) &&
  typeof value.started_at === "string" &&
  isOptionalString(value.finished_at) &&
  isOptionalString(value.error) &&
  (value.summary === undefined ||
    value.summary === null ||
    isRecord(value.summary));

export const isStep = (value: unknown): value is Step =>
  isRecord(value) &&
  typeof value.id === "number" &&
  typeof value.run_id === "string" &&
  typeof value.step === "string" &&
  isStepStatus(value.status) &&
  typeof value.started_at === "string" &&
  typeof value.finished_at === "string" &&
  isOptionalString(value.error) &&
  isOptionalString(value.output_tail);

export const isRunList = (value: unknown): value is { runs: Run[] } =>
  isRecord(value) && isArrayOf(value.runs, isRun);

export const isRunDetail = (value: unknown): value is RunDetail =>
  isRecord(value) && isRun(value.run) && isArrayOf(value.steps, isStep);

export const isFlow = (value: unknown): value is Flow =>
  isRecord(value) &&
  typeof value.name === "string" &&
  typeof value.schedule === "string" &&
  typeof value.default_schedule === "string" &&
  typeof value.running === "boolean" &&
  (value.last_run === null ||
    value.last_run === undefined ||
    isRun(value.last_run));

export const isFlowList = (value: unknown): value is { flows: Flow[] } =>
  isRecord(value) && isArrayOf(value.flows, isFlow);

export const isFlowRunStarted = (value: unknown): value is { flow: string } =>
  isRecord(value) && typeof value.flow === "string";

const isPromptSummary = (value: unknown): value is PromptSummary =>
  isRecord(value) &&
  typeof value.name === "string" &&
  typeof value.overridden === "boolean" &&
  isOptionalString(value.updated_at);

export const isPromptList = (
  value: unknown,
): value is { prompts: PromptSummary[] } =>
  isRecord(value) && isArrayOf(value.prompts, isPromptSummary);

export const isPrompt = (value: unknown): value is Prompt =>
  isRecord(value) &&
  typeof value.name === "string" &&
  typeof value.content === "string" &&
  typeof value.default === "string" &&
  typeof value.overridden === "boolean" &&
  isOptionalString(value.updated_at);

const isPromptVersion = (value: unknown): value is PromptVersion =>
  isRecord(value) &&
  isStringOrNull(value.content) &&
  typeof value.deleted === "boolean" &&
  typeof value.created_at === "string" &&
  typeof value.created_by === "string";

export const isPromptHistory = (
  value: unknown,
): value is { versions: PromptVersion[] } =>
  isRecord(value) && isArrayOf(value.versions, isPromptVersion);

export const isParam = (value: unknown): value is Param =>
  isRecord(value) &&
  typeof value.key === "string" &&
  typeof value.value === "string" &&
  typeof value.default === "string" &&
  (value.kind === "model" || value.kind === "bool" || value.kind === "int") &&
  typeof value.description === "string" &&
  typeof value.overridden === "boolean";

export const isTokenStatus = (value: unknown): value is TokenStatus =>
  isRecord(value) &&
  typeof value.set === "boolean" &&
  isOptionalString(value.updated_at);

export const isSettings = (value: unknown): value is Settings =>
  isRecord(value) &&
  isArrayOf(value.params, isParam) &&
  isTokenStatus(value.token);

const isFootprintProject = (value: unknown): value is FootprintProject =>
  isRecord(value) &&
  typeof value.key === "string" &&
  typeof value.memory_lines === "number" &&
  typeof value.memory_bytes === "number" &&
  typeof value.entries === "number" &&
  typeof value.pct_of_limit === "number";

const isFootprintGlobal = (value: unknown): value is FootprintGlobal =>
  isRecord(value) &&
  typeof value.claude_md_bytes === "number" &&
  typeof value.rules_unconditional_bytes === "number" &&
  typeof value.rules_scoped_bytes === "number" &&
  isStringArray(value.rules_unconditional) &&
  isStringArray(value.rules_scoped) &&
  typeof value.skills === "number";

const isFootprintLimits = (value: unknown): value is FootprintLimits =>
  isRecord(value) &&
  typeof value.memory_lines === "number" &&
  typeof value.memory_bytes === "number" &&
  typeof value.warn_pct === "number";

export const isFootprint = (value: unknown): value is Footprint =>
  isRecord(value) &&
  typeof value.generated_at === "string" &&
  isFootprintLimits(value.limits) &&
  isArrayOf(value.projects, isFootprintProject) &&
  isFootprintGlobal(value.global);

export const isProblem = (
  value: unknown,
): value is { title?: string; detail?: string } =>
  isRecord(value) &&
  (value.title === undefined || typeof value.title === "string") &&
  (value.detail === undefined || typeof value.detail === "string");
