import { useState } from "react";
import type { JSX } from "react";
import { errorMessage, resetSchedule, setSchedule } from "./api";
import { formatTime } from "./format";
import { StatusBadge } from "./StatusBadge";
import type { Flow } from "./types";

type Props = {
  flow: Flow;
  onRun: (name: string) => void;
  onChanged: (flow: Flow) => void;
};

export const FlowRow = ({ flow, onRun, onChanged }: Props): JSX.Element => {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(flow.schedule);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const save = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      onChanged(await setSchedule(flow.name, draft.trim()));
      setEditing(false);
    } catch (saveError: unknown) {
      setError(errorMessage(saveError));
    }
    setBusy(false);
  };

  const reset = async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const updated = await resetSchedule(flow.name);
      onChanged(updated);
      setDraft(updated.schedule);
      setEditing(false);
    } catch (resetError: unknown) {
      setError(errorMessage(resetError));
    }
    setBusy(false);
  };

  const lastRun = flow.last_run ?? null;

  return (
    <tr>
      <th scope="row">{flow.name}</th>
      <td>
        {editing ? (
          <div className="inline-form">
            <input
              aria-label={`Schedule for ${flow.name}`}
              value={draft}
              onChange={(event) => {
                setDraft(event.target.value);
              }}
              disabled={busy}
            />
            <button
              type="button"
              className="primary"
              onClick={() => {
                void save();
              }}
              disabled={busy}
            >
              Save
            </button>
            <button
              type="button"
              onClick={() => {
                setEditing(false);
                setDraft(flow.schedule);
                setError(null);
              }}
              disabled={busy}
            >
              Cancel
            </button>
          </div>
        ) : (
          <div className="inline-form">
            <code>{flow.schedule}</code>
            <button
              type="button"
              aria-label={`Edit schedule for ${flow.name}`}
              onClick={() => {
                setDraft(flow.schedule);
                setEditing(true);
              }}
            >
              Edit
            </button>
            <button
              type="button"
              aria-label={`Reset schedule for ${flow.name}`}
              onClick={() => {
                void reset();
              }}
              disabled={busy || flow.schedule === flow.default_schedule}
            >
              Reset
            </button>
          </div>
        )}
        {error !== null && (
          <p className="error-text" role="alert">
            {error}
          </p>
        )}
      </td>
      <td>
        {flow.running ? (
          <StatusBadge status="running" />
        ) : (
          <span className="hint">idle</span>
        )}
      </td>
      <td>
        {lastRun === null ? (
          <span className="hint">never</span>
        ) : (
          <a href={`#/runs/${encodeURIComponent(lastRun.id)}`}>
            <StatusBadge status={lastRun.status} />{" "}
            {formatTime(lastRun.started_at)}
          </a>
        )}
      </td>
      <td>
        <button
          type="button"
          aria-label={`Run ${flow.name} now`}
          onClick={() => {
            onRun(flow.name);
          }}
          disabled={flow.running}
        >
          Run now
        </button>
      </td>
    </tr>
  );
};
