export declare const backend: {
  b: (args: { job_id: string; reason?: string }) => Promise<unknown>;
  c: (args: Record<string, never>) => Promise<unknown>;
  d: (args: { job_id: string; approved?: string }) => Promise<unknown>;
};
