import * as React from 'react';
import type { JSX } from 'react';
import ReactMarkdown from 'react-markdown';
import type { SuspendedRun } from './types';

type Props = {
  run: SuspendedRun | null;
  approved: string;
  busy: boolean;
  onApprovedChange: (value: string) => void;
  onResume: () => void;
  onCancel: () => void;
};

export const RunDetail = ({
  run,
  approved,
  busy,
  onApprovedChange,
  onResume,
  onCancel,
}: Props): JSX.Element => {
  if (run === null) {
    return (
      <div className="detail">
        <p>Select a run to review its staged proposals.</p>
      </div>
    );
  }

  return (
    <div className="detail">
      <div className="markdown">
        <ReactMarkdown>{run.markdown}</ReactMarkdown>
      </div>
      <div className="actions">
        <label htmlFor="approved-input">approved:</label>
        <input
          id="approved-input"
          value={approved}
          onChange={(event) => onApprovedChange(event.target.value)}
          placeholder="all | none | group,names"
        />
        <button type="button" className="primary" onClick={onResume} disabled={busy}>
          Resume
        </button>
        <button type="button" className="danger" onClick={onCancel} disabled={busy}>
          Cancel run
        </button>
      </div>
    </div>
  );
};
