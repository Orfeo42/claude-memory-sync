import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { FootprintPage } from "./FootprintPage";
import type { Footprint } from "./types";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getFootprint: vi.fn(),
}));

const footprint: Footprint = {
  generated_at: "2026-11-06T10:00:00Z",
  limits: { memory_lines: 200, memory_bytes: 25600, warn_pct: 80 },
  projects: [
    { key: "alpha", memory_lines: 50, memory_bytes: 5000, entries: 4, pct_of_limit: 25 },
    { key: "beta", memory_lines: 190, memory_bytes: 20000, entries: 30, pct_of_limit: 95 },
  ],
  global: {
    claude_md_bytes: 2048,
    rules_unconditional_bytes: 512,
    rules_scoped_bytes: 100,
    rules_unconditional: ["go.md"],
    rules_scoped: ["helm.md", "ts.md"],
    skills: 7,
  },
};

describe("FootprintPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("renders the global block and highlights projects at or above warn_pct", async () => {
    vi.mocked(api.getFootprint).mockResolvedValue(footprint);
    render(<FootprintPage />);
    expect(await screen.findByText(/2\.0 KiB/)).toBeInTheDocument();
    expect(screen.getByText("go.md")).toBeInTheDocument();
    expect(screen.getByText("helm.md")).toBeInTheDocument();
    expect(screen.getByText(/: 7/)).toBeInTheDocument();
    const beta = screen.getByRole("rowheader", { name: "beta" }).closest("tr");
    const alpha = screen.getByRole("rowheader", { name: "alpha" }).closest("tr");
    expect(beta).toHaveAttribute("data-warn", "true");
    expect(beta).toHaveClass("warn");
    expect(alpha).toHaveAttribute("data-warn", "false");
    expect(screen.getByText("190 / 200")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "beta usage" })).toHaveAttribute(
      "aria-valuenow",
      "95",
    );
  });

  it("shows an empty projects state", async () => {
    vi.mocked(api.getFootprint).mockResolvedValue({ ...footprint, projects: [] });
    render(<FootprintPage />);
    expect(await screen.findByText("No projects.")).toBeInTheDocument();
  });

  it("shows the error", async () => {
    vi.mocked(api.getFootprint).mockRejectedValue(new Error("nope"));
    render(<FootprintPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("nope");
  });
});
