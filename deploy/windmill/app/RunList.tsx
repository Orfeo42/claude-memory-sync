import * as React from "react";
import type { JSX } from "react";
import type { Decision, RunGroup } from "./types";
import { itemKey } from "./types";

type Props = {
  runs: readonly RunGroup[];
  decisions: Readonly<Record<string, Decision>>;
  selectedKey: string | null;
  busy: boolean;
  error: string | null;
  status: string | null;
  onSelect: (key: string) => void;
  onRefresh: () => void;
  onSubmit: (runId: string) => void;
  onCancelRun: (runId: string) => void;
};

const decisionBadge = (decision: Decision | undefined): string => {
  if (decision === "approved") {
    return "✓";
  }
  if (decision === "rejected") {
    return "✗";
  }
  return "·";
};

export const RunList = ({
  runs,
  decisions,
  selectedKey,
  busy,
  error,
  status,
  onSelect,
  onRefresh,
  onSubmit,
  onCancelRun,
}: Props): JSX.Element => (
  <div className="run-list">
    <h2>Approval inbox</h2>
    <button type="button" onClick={onRefresh} disabled={busy}>
      {busy ? "Working…" : "Refresh"}
    </button>
    {error !== null && <p className="error-text">{error}</p>}
    {status !== null && <p className="status-text">{status}</p>}
    {runs.length === 0 && !busy && <p>No proposals waiting for review.</p>}
    {runs.map((run) => {
      const decided = run.items.filter(
        (item) => decisions[itemKey(item)] !== undefined,
      ).length;
      const allDecided = decided === run.items.length;
      return (
        <div key={run.runId} className="run-group">
          <div className="run-group-header">
            <div>
              <strong>{run.flow}</strong>
              <br />
              <small>{new Date(run.startedAt).toLocaleString()}</small>
            </div>
            <small>
              {decided}/{run.items.length}
            </small>
          </div>
          {run.items.map((item) => {
            const key = itemKey(item);
            return (
              <div
                key={key}
                className={key === selectedKey ? "run-row selected" : "run-row"}
                onClick={() => {
                  onSelect(key);
                }}
              >
                <span
                  className={`decision decision-${decisions[key] ?? "none"}`}
                >
                  {decisionBadge(decisions[key])}
                </span>{" "}
                {item.group}{" "}
                <span className={`kind-badge kind-${item.kind}`}>
                  {item.kind}
                </span>
              </div>
            );
          })}
          <div className="run-group-actions">
            <button
              type="button"
              className="primary"
              disabled={busy || !allDecided}
              onClick={() => {
                onSubmit(run.runId);
              }}
            >
              Submit decisions
            </button>
            <button
              type="button"
              className="danger"
              disabled={busy}
              onClick={() => {
                onCancelRun(run.runId);
              }}
            >
              Cancel run
            </button>
          </div>
        </div>
      );
    })}
  </div>
);
