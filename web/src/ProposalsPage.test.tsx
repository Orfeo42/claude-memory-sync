import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { ProposalsPage } from "./ProposalsPage";
import { makeDetail, makeProposal } from "./testData";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listProposals: vi.fn(),
  getProposal: vi.fn(),
  decideProposal: vi.fn(),
  retryProposal: vi.fn(),
  chatProposal: vi.fn(),
  applyRevision: vi.fn(),
}));

const pending = makeProposal({ id: "p1", name: "Deploy skill" });
const failed = makeProposal({
  id: "p2",
  name: "Broken rule",
  status: "failed",
  error: "push rejected",
});
const applying = makeProposal({
  id: "p3",
  name: "Running one",
  status: "applying",
});

describe("ProposalsPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.listProposals).mockResolvedValue([pending, failed, applying]);
    vi.mocked(api.getProposal).mockImplementation((id) =>
      Promise.resolve(
        makeDetail({ id, name: id === "p2" ? "Broken rule" : "Deploy skill", status: id === "p2" ? "failed" : "pending" }),
      ),
    );
  });

  it("renders pending, failed and in-progress sections", async () => {
    render(<ProposalsPage />);
    const pendingSection = await screen.findByRole("region", {
      name: "Pending",
    });
    expect(within(pendingSection).getByText(/Deploy skill/)).toBeInTheDocument();
    const failedSection = screen.getByRole("region", { name: "Failed" });
    expect(within(failedSection).getByText("push rejected")).toBeInTheDocument();
    const progress = screen.getByRole("region", { name: "In progress" });
    expect(within(progress).getByText(/Running one/)).toBeInTheDocument();
  });

  it("shows an empty state", async () => {
    vi.mocked(api.listProposals).mockResolvedValue([]);
    render(<ProposalsPage />);
    expect(await screen.findByText("No proposals waiting.")).toBeInTheDocument();
  });

  it("shows a load error", async () => {
    vi.mocked(api.listProposals).mockRejectedValue(new Error("boom"));
    render(<ProposalsPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  });

  it("renders the files and history of the selected proposal", async () => {
    const user = userEvent.setup();
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    expect(await screen.findByText("Deploy steps")).toBeInTheDocument();
    expect(screen.getByText("History (1)")).toBeInTheDocument();
  });

  it("approves with a reason", async () => {
    const user = userEvent.setup();
    vi.mocked(api.decideProposal).mockResolvedValue({
      proposal: makeProposal({ status: "approved" }),
      apply_started: true,
    });
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    await user.type(await screen.findByLabelText("Reason"), "looks good");
    await user.click(screen.getByRole("button", { name: "Approve" }));
    expect(api.decideProposal).toHaveBeenCalledWith("p1", "approve", "looks good");
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Proposal approved, apply started",
    );
  });

  it("rejects without a reason", async () => {
    const user = userEvent.setup();
    vi.mocked(api.decideProposal).mockResolvedValue({
      proposal: makeProposal({ status: "rejected" }),
      apply_started: false,
    });
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    expect(api.decideProposal).toHaveBeenCalledWith("p1", "reject", "");
    expect(await screen.findByRole("status")).toHaveTextContent("Proposal rejected");
  });

  it("shows the API error when a decision fails", async () => {
    const user = userEvent.setup();
    vi.mocked(api.decideProposal).mockRejectedValue(new Error("not pending"));
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    await user.click(await screen.findByRole("button", { name: "Approve" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("not pending");
  });

  it("retries a failed proposal", async () => {
    const user = userEvent.setup();
    vi.mocked(api.retryProposal).mockResolvedValue({
      proposal: makeProposal({ id: "p2", status: "approved" }),
      apply_started: true,
    });
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Broken rule/ }));
    await user.click(await screen.findByRole("button", { name: "Retry" }));
    expect(api.retryProposal).toHaveBeenCalledWith("p2");
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Proposal retried, apply started",
    );
  });

  it("sends a chat message with history and the chosen model", async () => {
    const user = userEvent.setup();
    vi.mocked(api.chatProposal).mockResolvedValue("first reply");
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    await user.selectOptions(await screen.findByLabelText("Model"), "opus");
    await user.type(screen.getByLabelText("Message"), "shorten it");
    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(api.chatProposal).toHaveBeenCalledWith("p1", "shorten it", "opus", []);
    expect(await screen.findByText("first reply")).toBeInTheDocument();

    vi.mocked(api.chatProposal).mockResolvedValue("second reply");
    await user.type(screen.getByLabelText("Message"), "again");
    await user.click(screen.getByRole("button", { name: "Send" }));
    expect(api.chatProposal).toHaveBeenLastCalledWith("p1", "again", "opus", [
      { role: "user", content: "shorten it" },
      { role: "assistant", content: "first reply" },
    ]);
  });

  it("applies the last assistant revision", async () => {
    const user = userEvent.setup();
    const reply = "===FILE: global/skills/deploy.md===\nnew\n===END===";
    vi.mocked(api.chatProposal).mockResolvedValue(reply);
    vi.mocked(api.applyRevision).mockResolvedValue({
      applied: ["global/skills/deploy.md"],
      proposal: makeProposal({
        files: [{ target_path: "global/skills/deploy.md", content: "revised body" }],
      }),
    });
    render(<ProposalsPage />);
    await user.click(await screen.findByRole("button", { name: /Deploy skill/ }));
    expect(screen.queryByRole("button", { name: "Apply revision" })).toBeNull();
    await user.type(await screen.findByLabelText("Message"), "rewrite");
    await user.click(screen.getByRole("button", { name: "Send" }));
    await user.click(await screen.findByRole("button", { name: "Apply revision" }));
    expect(api.applyRevision).toHaveBeenCalledWith("p1", reply);
    expect(await screen.findByText("revised body")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Revision applied to 1 file(s)",
    );
  });
});
