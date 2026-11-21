import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "./api";
import { SettingsPage } from "./SettingsPage";
import { makeParam } from "./testData";

vi.mock("./api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./api")>()),
  getSettings: vi.fn(),
  saveParam: vi.fn(),
  resetParam: vi.fn(),
  saveToken: vi.fn(),
  clearToken: vi.fn(),
}));

describe("SettingsPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(api.getSettings).mockResolvedValue({
      params: [
        makeParam(),
        makeParam({
          key: "staleness_enabled",
          kind: "bool",
          value: "false",
          default: "false",
        }),
        makeParam({
          key: "hotcache_max_bytes",
          kind: "int",
          value: "4000",
          default: "4000",
        }),
      ],
      token: { set: false, updated_at: null },
    });
  });

  it("saves a model param", async () => {
    const user = userEvent.setup();
    vi.mocked(api.saveParam).mockResolvedValue(
      makeParam({ value: "opus", overridden: true }),
    );
    render(<SettingsPage />);
    await user.selectOptions(await screen.findByLabelText("synth_model"), "opus");
    await user.click(screen.getByRole("button", { name: "Save synth_model" }));
    expect(api.saveParam).toHaveBeenCalledWith("synth_model", "opus");
    expect(await screen.findByText("overridden")).toBeInTheDocument();
  });

  it("saves a bool param as a string", async () => {
    const user = userEvent.setup();
    vi.mocked(api.saveParam).mockResolvedValue(
      makeParam({ key: "staleness_enabled", kind: "bool", value: "true", overridden: true }),
    );
    render(<SettingsPage />);
    await user.click(await screen.findByLabelText("staleness_enabled"));
    await user.click(screen.getByRole("button", { name: "Save staleness_enabled" }));
    expect(api.saveParam).toHaveBeenCalledWith("staleness_enabled", "true");
  });

  it("shows an inline validation error for an int param", async () => {
    const user = userEvent.setup();
    vi.mocked(api.saveParam).mockRejectedValue(
      new api.ApiError("hotcache_max_bytes must be at least 1000", 422),
    );
    render(<SettingsPage />);
    const input = await screen.findByLabelText("hotcache_max_bytes");
    await user.clear(input);
    await user.type(input, "5");
    await user.click(screen.getByRole("button", { name: "Save hotcache_max_bytes" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "hotcache_max_bytes must be at least 1000",
    );
  });

  it("resets an overridden param", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getSettings).mockResolvedValue({
      params: [makeParam({ value: "opus", overridden: true })],
      token: { set: false, updated_at: null },
    });
    vi.mocked(api.resetParam).mockResolvedValue(makeParam());
    render(<SettingsPage />);
    await user.click(await screen.findByRole("button", { name: "Reset synth_model" }));
    expect(api.resetParam).toHaveBeenCalledWith("synth_model");
    expect(await screen.findByLabelText("synth_model")).toHaveValue("sonnet");
  });

  it("saves the token, clears the input and never renders the value", async () => {
    const user = userEvent.setup();
    const secret = "sk-ant-oat01-secret-value";
    vi.mocked(api.saveToken).mockResolvedValue({
      set: true,
      updated_at: "2026-11-05T10:00:00Z",
    });
    const view = render(<SettingsPage />);
    expect(await screen.findByText("Token is not set")).toBeInTheDocument();
    const input = screen.getByLabelText("New token");
    expect(input).toHaveAttribute("type", "password");
    await user.type(input, secret);
    await user.click(screen.getByRole("button", { name: "Save token" }));
    expect(api.saveToken).toHaveBeenCalledWith(secret);
    expect(await screen.findByText(/Token is set/)).toBeInTheDocument();
    expect(screen.getByLabelText("New token")).toHaveValue("");
    expect(view.container.innerHTML).not.toContain(secret);
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });

  it("clears the token", async () => {
    const user = userEvent.setup();
    vi.mocked(api.getSettings).mockResolvedValue({
      params: [],
      token: { set: true, updated_at: "2026-11-05T10:00:00Z" },
    });
    vi.mocked(api.clearToken).mockResolvedValue({ set: false, updated_at: null });
    render(<SettingsPage />);
    await user.click(await screen.findByRole("button", { name: "Clear token" }));
    expect(await screen.findByText("Token is not set")).toBeInTheDocument();
  });

  it("shows token save errors without echoing the value", async () => {
    const user = userEvent.setup();
    vi.mocked(api.saveToken).mockRejectedValue(new api.ApiError("token rejected", 422));
    render(<SettingsPage />);
    await user.type(await screen.findByLabelText("New token"), "abc123");
    await user.click(screen.getByRole("button", { name: "Save token" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("token rejected");
  });

  it("shows a load error", async () => {
    vi.mocked(api.getSettings).mockRejectedValue(new Error("down"));
    render(<SettingsPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("down");
  });
});
