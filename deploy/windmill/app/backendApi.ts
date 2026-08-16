import { backend } from "./wmill";

export const listSuspended = (): Promise<unknown> => backend.c({});

export const resumeRun = (jobId: string, approved: string): Promise<unknown> =>
  backend.d({ job_id: jobId, approved });

export const cancelRun = (jobId: string, reason: string): Promise<unknown> =>
  backend.b({ job_id: jobId, reason });
