import * as React from 'react';
import { useCallback, useEffect, useState } from 'react';
import type { JSX } from 'react';
import { backend } from './wmill';
import { RunDetail } from './RunDetail';
import { RunList } from './RunList';
import { errorMessage, isSuspendedRun } from './types';
import type { SuspendedRun } from './types';

export const App = (): JSX.Element => {
  const [runs, setRuns] = useState<readonly SuspendedRun[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [approved, setApproved] = useState('all');
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const result: unknown = await backend.list_suspended({});
      const rows = Array.isArray(result) ? result.filter(isSuspendedRun) : [];
      setRuns(rows);
      setSelectedId((current) =>
        rows.some((row) => row.job_id === current) ? current : null,
      );
    } catch (loadError: unknown) {
      setError(errorMessage(loadError));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const resume = useCallback(async (): Promise<void> => {
    if (selectedId === null) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await backend.resume_run({ job_id: selectedId, approved });
      setStatus(`Resumed with approved=${approved}`);
      setApproved('all');
      await refresh();
    } catch (resumeError: unknown) {
      setError(errorMessage(resumeError));
      setBusy(false);
    }
  }, [selectedId, approved, refresh]);

  const cancel = useCallback(async (): Promise<void> => {
    if (selectedId === null) {
      return;
    }
    if (!window.confirm('Cancel this run and discard the whole batch?')) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await backend.cancel_run({
        job_id: selectedId,
        reason: 'rejected from approval inbox',
      });
      setStatus('Run canceled, batch discarded');
      await refresh();
    } catch (cancelError: unknown) {
      setError(errorMessage(cancelError));
      setBusy(false);
    }
  }, [selectedId, refresh]);

  const selected = runs.find((row) => row.job_id === selectedId) ?? null;

  return (
    <div className="page">
      <RunList
        runs={runs}
        selectedId={selectedId}
        busy={busy}
        error={error}
        status={status}
        onSelect={setSelectedId}
        onRefresh={() => void refresh()}
      />
      <RunDetail
        run={selected}
        approved={approved}
        busy={busy}
        onApprovedChange={setApproved}
        onResume={() => void resume()}
        onCancel={() => void cancel()}
      />
    </div>
  );
};

export default App;
