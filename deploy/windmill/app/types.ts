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
  decided_at: string | null;
  decided_by: string | null;
  claimed_by: string | null;
  claimed_until: string | null;
  applied_sha: string | null;
  error: string | null;
  files: ProposalFile[];
};

export type ProposalEvent = {
  id: number;
  proposal_id: string;
  from_status: string | null;
  to_status: string;
  actor: string;
  note: string | null;
  at: string;
};

export type ProposalDetail = {
  proposal: Proposal;
  events: ProposalEvent[];
};

export type DecideResult = {
  proposal: Proposal;
  apply_job_id: string | null;
};

export type RevisionResult = {
  applied: string[];
  proposal: Proposal;
};

export type ReplyPayload = { reply: string };

export type ChatMessage = {
  id: number;
  role: "user" | "assistant";
  content: string;
};

export const MODELS = ["sonnet", "opus", "haiku"] as const;
export type Model = (typeof MODELS)[number];

const STATUSES: readonly string[] = [
  "pending",
  "approved",
  "applying",
  "applied",
  "rejected",
  "expired",
  "failed",
];

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null;

const isOptionalString = (value: unknown): value is string | null | undefined =>
  value === null || value === undefined || typeof value === "string";

export const isProposalStatus = (value: unknown): value is ProposalStatus =>
  typeof value === "string" && STATUSES.includes(value);

export const isProposalFile = (value: unknown): value is ProposalFile =>
  isRecord(value) &&
  typeof value.target_path === "string" &&
  typeof value.content === "string";

const isOptionalFiles = (value: unknown): boolean =>
  value === null ||
  value === undefined ||
  (Array.isArray(value) && value.every(isProposalFile));

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
  isOptionalFiles(value.files);

export const isProposalEvent = (value: unknown): value is ProposalEvent =>
  isRecord(value) &&
  typeof value.id === "number" &&
  typeof value.proposal_id === "string" &&
  isOptionalString(value.from_status) &&
  typeof value.to_status === "string" &&
  typeof value.actor === "string" &&
  isOptionalString(value.note) &&
  typeof value.at === "string";

export const isProposalDetail = (value: unknown): value is ProposalDetail =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  Array.isArray(value.events) &&
  value.events.every(isProposalEvent);

export const isDecideResult = (value: unknown): value is DecideResult =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  isOptionalString(value.apply_job_id);

export const isReplyPayload = (value: unknown): value is ReplyPayload =>
  isRecord(value) && typeof value.reply === "string";

export const isRevisionPayload = (value: unknown): value is RevisionResult =>
  isRecord(value) &&
  isProposal(value.proposal) &&
  Array.isArray(value.applied) &&
  value.applied.every((entry) => typeof entry === "string");

export const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);
