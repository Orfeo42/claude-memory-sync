import * as React from 'react';
import type { JSX } from 'react';
import type { SuspendedRun } from './types';

type Props = {
  runs: readonly SuspendedRun[];
  selectedId: string | null;
  busy: boolean;
  error: string | null;
  status: string | null;
  onSelect: (jobId: string) => void;
  onRefresh: () => void;
};

export const RunList = ({
  runs,
  selectedId,
  busy,
  error,
  status,
  onSelect,
  onRefresh,
}: Props): JSX.Element => (
  <div className="run-list">
    <h2>Approval inbox</h2>
    <button type="button" onClick={onRefresh} disabled={busy}>
      {busy ? 'Working…' : 'Refresh'}
    </button>
    {error !== null && <p className="error-text">{error}</p>}
    {status !== null && <p className="status-text">{status}</p>}
    {runs.length === 0 && !busy && <p>No runs waiting for approval.</p>}
    {runs.map((run) => (
      <div
        key={run.job_id}
        className={run.job_id === selectedId ? 'run-row selected' : 'run-row'}
        onClick={() => onSelect(run.job_id)}
      >
        <strong>{run.flow.replace('f/memory/', '')}</strong>
        <br />
        <small>{new Date(run.started_at).toLocaleString()}</small>
      </div>
    ))}
  </div>
);
