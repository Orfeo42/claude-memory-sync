import { backend } from "./wmill";

export const listSuspended = (): Promise<unknown> => backend.c({});

export const resumeRun = (jobId: string, approved: string): Promise<unknown> =>
  backend.d({ job_id: jobId, approved });

export const cancelRun = (jobId: string, reason: string): Promise<unknown> =>
  backend.b({ job_id: jobId, reason });

export const chatProposal = (
  runId: string,
  group: string,
  message: string,
  historyJson: string,
  model: string,
): Promise<unknown> =>
  backend.e({
    run_id: runId,
    group,
    message,
    history_json: historyJson,
    model,
  });

export const applyRevision = (
  runId: string,
  group: string,
  reply: string,
): Promise<unknown> => backend.f({ run_id: runId, group, reply });
