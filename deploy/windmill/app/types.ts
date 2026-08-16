export type SuspendedRun = {
  job_id: string;
  flow: string;
  started_at: string;
  markdown: string;
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null;

export const isSuspendedRun = (value: unknown): value is SuspendedRun =>
  isRecord(value) &&
  typeof value.job_id === 'string' &&
  typeof value.flow === 'string' &&
  typeof value.started_at === 'string' &&
  typeof value.markdown === 'string';

export const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);
