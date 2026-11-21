import {
  isChatReply,
  isDecideResult,
  isFlow,
  isFlowList,
  isFlowRunStarted,
  isFootprint,
  isParam,
  isProblem,
  isPrompt,
  isPromptHistory,
  isPromptList,
  isProposalDetail,
  isProposalList,
  isRevisionResult,
  isRunDetail,
  isRunList,
  isSettings,
  isTokenStatus,
} from "./types";
import type {
  ChatTurn,
  DecideResult,
  Flow,
  Footprint,
  Model,
  Param,
  Prompt,
  PromptSummary,
  PromptVersion,
  Proposal,
  ProposalDetail,
  RevisionResult,
  Run,
  RunDetail,
  Settings,
  TokenStatus,
} from "./types";

export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : "Unexpected error";

type Method = "GET" | "POST" | "PUT" | "DELETE";

const problemMessage = async (response: Response): Promise<string> => {
  const fallback = `Request failed (${String(response.status)})`;
  try {
    const body: unknown = await response.json();
    if (!isProblem(body)) {
      return fallback;
    }
    return body.detail ?? body.title ?? fallback;
  } catch {
    return fallback;
  }
};

const request = async (
  method: Method,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<unknown> => {
  const init: RequestInit = { method };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  if (signal !== undefined) {
    init.signal = signal;
  }
  let response: Response;
  try {
    response = await fetch(`/api${path}`, init);
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new ApiError("Network error: the server is unreachable", 0);
  }
  if (!response.ok) {
    throw new ApiError(await problemMessage(response), response.status);
  }
  try {
    return await response.json();
  } catch {
    throw new ApiError("Malformed response from the server", response.status);
  }
};

const malformed = (what: string): ApiError =>
  new ApiError(`Malformed ${what} response from the server`, 502);

const enc = encodeURIComponent;

export const listProposals = async (
  signal?: AbortSignal,
): Promise<Proposal[]> => {
  const data = await request("GET", "/proposals", undefined, signal);
  if (!isProposalList(data)) {
    throw malformed("proposals");
  }
  return data.proposals;
};

export const getProposal = async (
  id: string,
  signal?: AbortSignal,
): Promise<ProposalDetail> => {
  const data = await request("GET", `/proposals/${enc(id)}`, undefined, signal);
  if (!isProposalDetail(data)) {
    throw malformed("proposal");
  }
  return data;
};

export const decideProposal = async (
  id: string,
  decision: "approve" | "reject",
  reason: string,
): Promise<DecideResult> => {
  const body = reason === "" ? { decision } : { decision, reason };
  const data = await request("POST", `/proposals/${enc(id)}/decision`, body);
  if (!isDecideResult(data)) {
    throw malformed("decision");
  }
  return data;
};

export const retryProposal = async (id: string): Promise<DecideResult> => {
  const data = await request("POST", `/proposals/${enc(id)}/retry`);
  if (!isDecideResult(data)) {
    throw malformed("retry");
  }
  return data;
};

export const chatProposal = async (
  id: string,
  message: string,
  model: Model,
  history: readonly ChatTurn[],
): Promise<string> => {
  const data = await request("POST", `/proposals/${enc(id)}/chat`, {
    message,
    model,
    history,
  });
  if (!isChatReply(data)) {
    throw malformed("chat");
  }
  return data.reply;
};

export const applyRevision = async (
  id: string,
  reply: string,
): Promise<RevisionResult> => {
  const data = await request("POST", `/proposals/${enc(id)}/revision`, {
    reply,
  });
  if (!isRevisionResult(data)) {
    throw malformed("revision");
  }
  return data;
};

export const listFlows = async (signal?: AbortSignal): Promise<Flow[]> => {
  const data = await request("GET", "/flows", undefined, signal);
  if (!isFlowList(data)) {
    throw malformed("flows");
  }
  return data.flows;
};

export const runFlow = async (name: string): Promise<void> => {
  const data = await request("POST", `/flows/${enc(name)}/run`);
  if (!isFlowRunStarted(data)) {
    throw malformed("flow run");
  }
};

export const setSchedule = async (
  name: string,
  cron: string,
): Promise<Flow> => {
  const data = await request("PUT", `/flows/${enc(name)}/schedule`, { cron });
  if (!isFlow(data)) {
    throw malformed("schedule");
  }
  return data;
};

export const resetSchedule = async (name: string): Promise<Flow> => {
  const data = await request("DELETE", `/flows/${enc(name)}/schedule`);
  if (!isFlow(data)) {
    throw malformed("schedule");
  }
  return data;
};

export type RunQuery = {
  flow?: string;
  status?: string;
  before?: string;
  limit?: number;
};

export const listRuns = async (
  query: RunQuery,
  signal?: AbortSignal,
): Promise<Run[]> => {
  const params = new URLSearchParams();
  if (query.flow !== undefined) {
    params.set("flow", query.flow);
  }
  if (query.status !== undefined) {
    params.set("status", query.status);
  }
  if (query.before !== undefined) {
    params.set("before", query.before);
  }
  if (query.limit !== undefined) {
    params.set("limit", String(query.limit));
  }
  const suffix = params.size === 0 ? "" : `?${params.toString()}`;
  const data = await request("GET", `/runs${suffix}`, undefined, signal);
  if (!isRunList(data)) {
    throw malformed("runs");
  }
  return data.runs;
};

export const getRun = async (
  id: string,
  signal?: AbortSignal,
): Promise<RunDetail> => {
  const data = await request("GET", `/runs/${enc(id)}`, undefined, signal);
  if (!isRunDetail(data)) {
    throw malformed("run");
  }
  return data;
};

export const listPrompts = async (
  signal?: AbortSignal,
): Promise<PromptSummary[]> => {
  const data = await request("GET", "/prompts", undefined, signal);
  if (!isPromptList(data)) {
    throw malformed("prompts");
  }
  return data.prompts;
};

export const getPrompt = async (
  name: string,
  signal?: AbortSignal,
): Promise<Prompt> => {
  const data = await request("GET", `/prompts/${enc(name)}`, undefined, signal);
  if (!isPrompt(data)) {
    throw malformed("prompt");
  }
  return data;
};

export const savePrompt = async (
  name: string,
  content: string,
): Promise<Prompt> => {
  const data = await request("PUT", `/prompts/${enc(name)}`, { content });
  if (!isPrompt(data)) {
    throw malformed("prompt");
  }
  return data;
};

export const resetPrompt = async (name: string): Promise<Prompt> => {
  const data = await request("DELETE", `/prompts/${enc(name)}`);
  if (!isPrompt(data)) {
    throw malformed("prompt");
  }
  return data;
};

export const getPromptHistory = async (
  name: string,
  signal?: AbortSignal,
): Promise<PromptVersion[]> => {
  const data = await request(
    "GET",
    `/prompts/${enc(name)}/history`,
    undefined,
    signal,
  );
  if (!isPromptHistory(data)) {
    throw malformed("prompt history");
  }
  return data.versions;
};

export const getSettings = async (signal?: AbortSignal): Promise<Settings> => {
  const data = await request("GET", "/settings", undefined, signal);
  if (!isSettings(data)) {
    throw malformed("settings");
  }
  return data;
};

export const saveParam = async (key: string, value: string): Promise<Param> => {
  const data = await request("PUT", `/settings/params/${enc(key)}`, { value });
  if (!isParam(data)) {
    throw malformed("param");
  }
  return data;
};

export const resetParam = async (key: string): Promise<Param> => {
  const data = await request("DELETE", `/settings/params/${enc(key)}`);
  if (!isParam(data)) {
    throw malformed("param");
  }
  return data;
};

export const saveToken = async (token: string): Promise<TokenStatus> => {
  const data = await request("PUT", "/settings/token", { token });
  if (!isTokenStatus(data)) {
    throw malformed("token");
  }
  return data;
};

export const clearToken = async (): Promise<TokenStatus> => {
  const data = await request("DELETE", "/settings/token");
  if (!isTokenStatus(data)) {
    throw malformed("token");
  }
  return data;
};

export const getFootprint = async (signal?: AbortSignal): Promise<Footprint> => {
  const data = await request("GET", "/report/footprint", undefined, signal);
  if (!isFootprint(data)) {
    throw malformed("footprint");
  }
  return data;
};
