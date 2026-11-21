import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { App } from "./App";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listProposals: vi.fn(),
  getFootprint: vi.fn(),
  getSettings: vi.fn(),
}));

describe("App", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.listProposals).mockResolvedValue([]);
    vi.mocked(api.getSettings).mockResolvedValue({
      params: [],
      token: { set: false, updated_at: null },
    });
    window.location.hash = "";
  });

  it("renders the proposals tab by default and marks it current", async () => {
    render(<App />);
    expect(await screen.findByText("No proposals waiting.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Proposals" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("switches page when the hash changes", async () => {
    render(<App />);
    await screen.findByText("No proposals waiting.");
    act(() => {
      window.location.hash = "#/settings";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(await screen.findByText("No parameters.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});
