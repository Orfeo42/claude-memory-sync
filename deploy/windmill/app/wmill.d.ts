export declare const backend: {
  b: (args: { job_id: string; reason?: string }) => Promise<unknown>;
  c: (args: Record<string, never>) => Promise<unknown>;
  d: (args: { job_id: string; approved?: string }) => Promise<unknown>;
  e: (args: {
    run_id: string;
    group: string;
    message: string;
    history_json?: string;
    model?: string;
  }) => Promise<unknown>;
  f: (args: {
    run_id: string;
    group: string;
    reply: string;
  }) => Promise<unknown>;
};
