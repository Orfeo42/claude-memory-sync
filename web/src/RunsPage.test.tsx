import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { RunsPage } from "./RunsPage";
import { makeFlow, makeRun } from "./testData";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listFlows: vi.fn(),
  listRuns: vi.fn(),
  runFlow: vi.fn(),
  setSchedule: vi.fn(),
  resetSchedule: vi.fn(),
  getRun: vi.fn(),
}));

describe("RunsPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.listFlows).mockResolvedValue([
      makeFlow(),
      makeFlow({ name: "taskmine", running: true, last_run: null }),
    ]);
    vi.mocked(api.listRuns).mockResolvedValue([
      makeRun({ id: "run-2", flow: "taskmine", status: "failed" }),
      makeRun({ id: "run-1" }),
    ]);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders flows with running badge and last run", async () => {
    render(<RunsPage id={null} />);
    const header = await screen.findByRole("rowheader", { name: "taskmine" });
    const row = header.closest("tr");
    if (row === null) {
      throw new Error("flow row not found");
    }
    expect(within(row).getByText("running")).toBeInTheDocument();
    expect(within(row).getByText("never")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Run taskmine now" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Run intake now" })).toBeEnabled();
  });

  it("lists runs newest first with links to details", async () => {
    render(<RunsPage id={null} />);
    const links = await screen.findAllByRole("link", { name: /manual|schedule/ });
    expect(links[0]).toHaveAttribute("href", "#/runs/run-2");
  });

  it("shows empty states", async () => {
    vi.mocked(api.listFlows).mockResolvedValue([]);
    vi.mocked(api.listRuns).mockResolvedValue([]);
    render(<RunsPage id={null} />);
    expect(await screen.findByText("No flows.")).toBeInTheDocument();
    expect(await screen.findByText("No runs found.")).toBeInTheDocument();
  });

  it("starts a flow and refreshes", async () => {
    const user = userEvent.setup();
    vi.mocked(api.runFlow).mockResolvedValue();
    render(<RunsPage id={null} />);
    await user.click(await screen.findByRole("button", { name: "Run intake now" }));
    expect(api.runFlow).toHaveBeenCalledWith("intake");
    expect(await screen.findByRole("status")).toHaveTextContent("intake started");
    expect(api.listFlows).toHaveBeenCalledTimes(2);
  });

  it("shows a message when the flow is already running (409)", async () => {
    const user = userEvent.setup();
    vi.mocked(api.runFlow).mockRejectedValue(new api.ApiError("conflict", 409));
    render(<RunsPage id={null} />);
    await user.click(await screen.findByRole("button", { name: "Run intake now" }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "intake is already running",
    );
  });

  it("edits a schedule", async () => {
    const user = userEvent.setup();
    vi.mocked(api.setSchedule).mockResolvedValue(
      makeFlow({ schedule: "0 * * * *" }),
    );
    render(<RunsPage id={null} />);
    await user.click(
      await screen.findByRole("button", { name: "Edit schedule for intake" }),
    );
    const input = screen.getByLabelText("Schedule for intake");
    await user.clear(input);
    await user.type(input, "0 * * * *");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(api.setSchedule).toHaveBeenCalledWith("intake", "0 * * * *");
    expect(await screen.findByText("0 * * * *")).toBeInTheDocument();
  });

  it("shows an inline error for an invalid schedule", async () => {
    const user = userEvent.setup();
    vi.mocked(api.setSchedule).mockRejectedValue(new api.ApiError("bad cron", 422));
    render(<RunsPage id={null} />);
    await user.click(
      await screen.findByRole("button", { name: "Edit schedule for intake" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("bad cron");
  });

  it("resets a schedule to the default", async () => {
    const user = userEvent.setup();
    vi.mocked(api.listFlows).mockResolvedValue([
      makeFlow({ schedule: "0 * * * *" }),
    ]);
    vi.mocked(api.resetSchedule).mockResolvedValue(makeFlow());
    render(<RunsPage id={null} />);
    await user.click(
      await screen.findByRole("button", { name: "Reset schedule for intake" }),
    );
    expect(api.resetSchedule).toHaveBeenCalledWith("intake");
    expect(await screen.findByText("*/45 * * * *")).toBeInTheDocument();
  });

  it("filters runs by status", async () => {
    const user = userEvent.setup();
    render(<RunsPage id={null} />);
    await user.selectOptions(await screen.findByLabelText("Status"), "failed");
    expect(api.listRuns).toHaveBeenLastCalledWith(
      { limit: 50, status: "failed" },
      expect.any(AbortSignal),
    );
  });

  it("loads more runs using before", async () => {
    const user = userEvent.setup();
    const page = Array.from({ length: 50 }, (_, index) =>
      makeRun({ id: `run-${String(index)}`, started_at: `2026-11-02T09:00:${String(index).padStart(2, "0")}Z` }),
    );
    vi.mocked(api.listRuns).mockResolvedValueOnce(page);
    vi.mocked(api.listRuns).mockResolvedValueOnce([makeRun({ id: "older" })]);
    render(<RunsPage id={null} />);
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(api.listRuns).toHaveBeenLastCalledWith({
      limit: 50,
      before: "2026-11-02T09:00:49Z",
    });
    expect(await screen.findAllByRole("link", { name: /manual|schedule/ })).toHaveLength(51);
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("refreshes flows every 10 seconds while visible and stops on unmount", async () => {
    vi.useFakeTimers();
    const view = render(<RunsPage id={null} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(api.listFlows).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(api.listFlows).toHaveBeenCalledTimes(2);
    view.unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(api.listFlows).toHaveBeenCalledTimes(2);
  });

  it("skips the refresh while the tab is hidden", async () => {
    vi.useFakeTimers();
    const visibility = vi
      .spyOn(document, "visibilityState", "get")
      .mockReturnValue("hidden");
    render(<RunsPage id={null} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(api.listFlows).toHaveBeenCalledTimes(1);
    visibility.mockRestore();
  });
});
