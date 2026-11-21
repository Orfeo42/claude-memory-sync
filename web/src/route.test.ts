import { describe, expect, it } from "vitest";
import { parseRoute } from "./route";

describe("parseRoute", () => {
  it("defaults to proposals", () => {
    expect(parseRoute("")).toEqual({ tab: "proposals", id: null });
  });

  it("falls back to proposals for an unknown tab", () => {
    expect(parseRoute("#/nope")).toEqual({ tab: "proposals", id: null });
  });

  it("parses a tab", () => {
    expect(parseRoute("#/settings")).toEqual({ tab: "settings", id: null });
  });

  it("parses a tab with an id", () => {
    expect(parseRoute("#/runs/abc-1")).toEqual({ tab: "runs", id: "abc-1" });
  });

  it("decodes the id", () => {
    expect(parseRoute("#/prompts/a%20b")).toEqual({ tab: "prompts", id: "a b" });
  });

  it("ignores an undecodable id", () => {
    expect(parseRoute("#/prompts/%E0%A4%A")).toEqual({ tab: "prompts", id: null });
  });
});
