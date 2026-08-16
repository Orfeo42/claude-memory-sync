export type ProposalItem = {
  run_id: string;
  flow: string;
  started_at: string;
  group: string;
  kind: string;
  content: string;
};

export type Decision = "approved" | "rejected";

export type RunGroup = {
  runId: string;
  flow: string;
  startedAt: string;
  items: readonly ProposalItem[];
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null;

export const isProposalItem = (value: unknown): value is ProposalItem =>
  isRecord(value) &&
  typeof value.run_id === "string" &&
  typeof value.flow === "string" &&
  typeof value.started_at === "string" &&
  typeof value.group === "string" &&
  typeof value.kind === "string" &&
  typeof value.content === "string";

export const itemKey = (item: ProposalItem): string =>
  `${item.run_id}::${item.group}`;

export const groupByRun = (items: readonly ProposalItem[]): RunGroup[] => {
  const groups = new Map<string, RunGroup>();
  for (const item of items) {
    const existing = groups.get(item.run_id);
    if (existing === undefined) {
      groups.set(item.run_id, {
        runId: item.run_id,
        flow: item.flow,
        startedAt: item.started_at,
        items: [item],
      });
    } else {
      groups.set(item.run_id, {
        ...existing,
        items: [...existing.items, item],
      });
    }
  }
  return [...groups.values()];
};

export type ChatMessage = {
  role: "user" | "assistant";
  content: string;
};

export const isReplyPayload = (value: unknown): value is { reply: string } =>
  isRecord(value) && typeof value.reply === "string";

export const isRevisionPayload = (
  value: unknown,
): value is { applied: string[]; content: string } =>
  isRecord(value) &&
  Array.isArray(value.applied) &&
  typeof value.content === "string";

export const MODELS = ["sonnet", "opus", "haiku"] as const;
export type Model = (typeof MODELS)[number];

export const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);
