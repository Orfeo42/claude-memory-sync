import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { RunDetailView } from "./RunDetailView";
import { makeRun, makeStep } from "./testData";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getRun: vi.fn(),
}));

describe("RunDetailView", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("renders status, summary and steps including lossy", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getRun).mockResolvedValue({
      run: makeRun({ status: "partial", error: "one client failed" }),
      steps: [
        makeStep({ id: 1, step: "scan" }),
        makeStep({
          id: 2,
          step: "merge",
          status: "lossy",
          output_tail: "dropped 2 lines",
        }),
      ],
    });
    render(<RunDetailView id="run-1" />);
    expect(await screen.findByText("partial")).toBeInTheDocument();
    expect(screen.getByText("one client failed")).toBeInTheDocument();
    expect(screen.getByText("clients")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("lossy")).toBeInTheDocument();
    expect(screen.getByText("scan")).toBeInTheDocument();
    await user.click(screen.getByText("Output"));
    expect(screen.getByText("dropped 2 lines").tagName).toBe("PRE");
  });

  it("shows empty summary and steps", async () => {
    vi.mocked(api.getRun).mockResolvedValue({
      run: makeRun({ summary: {} }),
      steps: [],
    });
    render(<RunDetailView id="run-1" />);
    expect(await screen.findByText("No summary.")).toBeInTheDocument();
    expect(screen.getByText("No steps recorded.")).toBeInTheDocument();
  });

  it("shows the API error", async () => {
    vi.mocked(api.getRun).mockRejectedValue(new api.ApiError("run not found", 404));
    render(<RunDetailView id="nope" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("run not found");
  });
});
