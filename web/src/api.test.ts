import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  decideProposal,
  getFootprint,
  getRun,
  getSettings,
  listFlows,
  listProposals,
  listRuns,
  runFlow,
  saveToken,
  savePrompt,
} from "./api";
import { makeFlow, makeProposal, makeRun, makeStep } from "./testData";

const fetchMock = vi.fn<typeof fetch>();

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const lastCall = (): { url: string; init: RequestInit } => {
  const call = fetchMock.mock.calls.at(-1);
  if (call === undefined || typeof call[0] !== "string") {
    throw new Error("fetch was not called with a string url");
  }
  return { url: call[0], init: call[1] ?? {} };
};

describe("api", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("response guards", () => {
    it("accepts a valid proposals list", async () => {
      fetchMock.mockResolvedValue(json({ proposals: [makeProposal()] }));
      const proposals = await listProposals();
      expect(proposals).toHaveLength(1);
      expect(lastCall().url).toBe("/api/proposals");
    });

    it("rejects a bare array instead of a wrapped list", async () => {
      fetchMock.mockResolvedValue(json([makeProposal()]));
      await expect(listProposals()).rejects.toThrow("Malformed proposals");
    });

    it("rejects a proposal with an unknown status", async () => {
      fetchMock.mockResolvedValue(
        json({ proposals: [{ ...makeProposal(), status: "weird" }] }),
      );
      await expect(listProposals()).rejects.toBeInstanceOf(ApiError);
    });

    it("rejects a decision result without apply_started", async () => {
      fetchMock.mockResolvedValue(json({ proposal: makeProposal() }));
      await expect(decideProposal("p1", "approve", "")).rejects.toThrow(
        "Malformed decision",
      );
    });

    it("rejects flows with a wrong field type", async () => {
      fetchMock.mockResolvedValue(
        json({ flows: [{ ...makeFlow(), running: "yes" }] }),
      );
      await expect(listFlows()).rejects.toThrow("Malformed flows");
    });

    it("rejects a run detail whose steps are malformed", async () => {
      fetchMock.mockResolvedValue(
        json({ run: makeRun(), steps: [{ ...makeStep(), status: "bogus" }] }),
      );
      await expect(getRun("run-1")).rejects.toThrow("Malformed run");
    });

    it("rejects settings with an unknown param kind", async () => {
      fetchMock.mockResolvedValue(
        json({
          params: [
            {
              key: "k",
              value: "v",
              default: "v",
              kind: "float",
              description: "d",
              overridden: false,
            },
          ],
          token: { set: false, updated_at: null },
        }),
      );
      await expect(getSettings()).rejects.toThrow("Malformed settings");
    });

    it("rejects a footprint missing the global block", async () => {
      fetchMock.mockResolvedValue(
        json({
          generated_at: "2026-11-06T10:00:00Z",
          limits: { memory_lines: 200, memory_bytes: 25600, warn_pct: 80 },
          projects: [],
        }),
      );
      await expect(getFootprint()).rejects.toThrow("Malformed footprint");
    });

    it("rejects a non-JSON success body", async () => {
      fetchMock.mockResolvedValue(new Response("<html>", { status: 200 }));
      await expect(listFlows()).rejects.toThrow("Malformed response");
    });
  });

  describe("errors", () => {
    it("surfaces the problem detail", async () => {
      fetchMock.mockResolvedValue(
        json({ title: "Conflict", status: 409, detail: "flow is running" }, 409),
      );
      const error = await runFlow("intake").catch((caught: unknown) => caught);
      expect(error).toBeInstanceOf(ApiError);
      expect(error).toMatchObject({ message: "flow is running", status: 409 });
    });

    it("falls back to the problem title", async () => {
      fetchMock.mockResolvedValue(json({ title: "Bad Request", status: 400 }, 400));
      await expect(savePrompt("p", "x")).rejects.toThrow("Bad Request");
    });

    it("falls back to the status when the error body is not a problem", async () => {
      fetchMock.mockResolvedValue(new Response("oops", { status: 500 }));
      await expect(listFlows()).rejects.toThrow("Request failed (500)");
    });

    it("reports a network failure", async () => {
      fetchMock.mockRejectedValue(new TypeError("failed to fetch"));
      await expect(listFlows()).rejects.toThrow("Network error");
    });
  });

  describe("requests", () => {
    it("posts the decision body", async () => {
      fetchMock.mockResolvedValue(
        json({ proposal: makeProposal(), apply_started: true }),
      );
      const result = await decideProposal("p1", "reject", "no");
      expect(result.apply_started).toBe(true);
      const { url, init } = lastCall();
      expect(url).toBe("/api/proposals/p1/decision");
      expect(init.method).toBe("POST");
      expect(init.body).toBe(JSON.stringify({ decision: "reject", reason: "no" }));
    });

    it("omits an empty reason", async () => {
      fetchMock.mockResolvedValue(
        json({ proposal: makeProposal(), apply_started: false }),
      );
      await decideProposal("p1", "approve", "");
      expect(lastCall().init.body).toBe(JSON.stringify({ decision: "approve" }));
    });

    it("builds the runs query string", async () => {
      fetchMock.mockResolvedValue(json({ runs: [] }));
      await listRuns({ flow: "intake", status: "failed", before: "t", limit: 10 });
      expect(lastCall().url).toBe(
        "/api/runs?flow=intake&status=failed&before=t&limit=10",
      );
    });

    it("sends the token only in the request body", async () => {
      fetchMock.mockResolvedValue(json({ set: true, updated_at: null }));
      await saveToken("secret-token");
      const { url, init } = lastCall();
      expect(url).toBe("/api/settings/token");
      expect(url).not.toContain("secret-token");
      expect(init.method).toBe("PUT");
      expect(init.body).toBe(JSON.stringify({ token: "secret-token" }));
    });

    it("encodes path parameters", async () => {
      fetchMock.mockResolvedValue(json({ flow: "a b" }, 202));
      await runFlow("a b");
      expect(lastCall().url).toBe("/api/flows/a%20b/run");
    });
  });
});
