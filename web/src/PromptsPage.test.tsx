import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { PromptsPage } from "./PromptsPage";
import { makePrompt } from "./testData";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  listPrompts: vi.fn(),
  getPrompt: vi.fn(),
  savePrompt: vi.fn(),
  resetPrompt: vi.fn(),
  getPromptHistory: vi.fn(),
}));

describe("PromptsPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.getPromptHistory).mockResolvedValue([]);
  });

  it("lists prompts with the overridden badge", async () => {
    vi.mocked(api.listPrompts).mockResolvedValue([
      { name: "intake-judge", overridden: true, updated_at: "2026-11-03T10:00:00Z" },
      { name: "synth-condense", overridden: false, updated_at: null },
    ]);
    render(<PromptsPage name={null} />);
    expect(await screen.findByRole("link", { name: "intake-judge" })).toHaveAttribute(
      "href",
      "#/prompts/intake-judge",
    );
    expect(screen.getAllByText("overridden")).toHaveLength(1);
  });

  it("shows an empty list", async () => {
    vi.mocked(api.listPrompts).mockResolvedValue([]);
    render(<PromptsPage name={null} />);
    expect(await screen.findByText("No prompts.")).toBeInTheDocument();
  });

  it("saves an edited prompt and reloads the history", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getPrompt).mockResolvedValue(makePrompt());
    vi.mocked(api.savePrompt).mockResolvedValue(
      makePrompt({ content: "new text", overridden: true }),
    );
    render(<PromptsPage name="intake-judge" />);
    const editor = await screen.findByLabelText("Prompt content");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await user.clear(editor);
    await user.type(editor, "new text");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(api.savePrompt).toHaveBeenCalledWith("intake-judge", "new text");
    expect(await screen.findByRole("status")).toHaveTextContent("Saved");
    expect(api.getPromptHistory).toHaveBeenCalledTimes(2);
  });

  it("shows the API error on save", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getPrompt).mockResolvedValue(makePrompt());
    vi.mocked(api.savePrompt).mockRejectedValue(new api.ApiError("too long", 422));
    render(<PromptsPage name="intake-judge" />);
    await user.type(await screen.findByLabelText("Prompt content"), "x");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("too long");
  });

  it("resets to default after an in-page confirmation", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getPrompt).mockResolvedValue(
      makePrompt({ content: "custom", overridden: true }),
    );
    vi.mocked(api.resetPrompt).mockResolvedValue(makePrompt());
    render(<PromptsPage name="intake-judge" />);
    await user.click(await screen.findByRole("button", { name: "Reset to default" }));
    expect(api.resetPrompt).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Reset to default" }));
    await user.click(screen.getByRole("button", { name: "Confirm reset" }));
    expect(api.resetPrompt).toHaveBeenCalledWith("intake-judge");
    expect(await screen.findByRole("status")).toHaveTextContent("Reset to default");
    expect(screen.getByLabelText("Prompt content")).toHaveValue("default text");
  });

  it("toggles the default view and renders history newest first", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getPrompt).mockResolvedValue(
      makePrompt({ content: "custom", default: "the default body", overridden: true }),
    );
    vi.mocked(api.getPromptHistory).mockResolvedValue([
      { content: null, deleted: true, created_at: "2026-11-04T10:00:00Z", created_by: "ui" },
      { content: "older body", deleted: false, created_at: "2026-11-03T10:00:00Z", created_by: "ui" },
    ]);
    render(<PromptsPage name="intake-judge" />);
    await user.click(await screen.findByRole("button", { name: "Default" }));
    expect(screen.getByLabelText("Default prompt")).toHaveTextContent("the default body");
    const items = screen.getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Reset to default");
    expect(items[1]).toHaveTextContent("Content");
  });
});
