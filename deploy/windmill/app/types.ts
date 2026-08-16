export type ProposalItem = {
  run_id: string;
  flow: string;
  started_at: string;
  group: string;
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

export const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);
