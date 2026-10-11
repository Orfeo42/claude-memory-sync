import * as React from "react";
import type { JSX } from "react";
import type { Proposal } from "./types";

type Props = {
  pending: readonly Proposal[];
  failed: readonly Proposal[];
  inProgress: readonly Proposal[];
  selectedId: string | null;
  busy: boolean;
  error: string | null;
  status: string | null;
  onSelect: (id: string) => void;
  onRefresh: () => void;
};

type SectionProps = {
  title: string;
  proposals: readonly Proposal[];
  selectedId: string | null;
  onSelect: (id: string) => void;
};

const Section = ({
  title,
  proposals,
  selectedId,
  onSelect,
}: SectionProps): JSX.Element | null => {
  if (proposals.length === 0) {
    return null;
  }
  return (
    <div className="run-group">
      <div className="run-group-header">
        <strong>{title}</strong>
        <small>{proposals.length}</small>
      </div>
      {proposals.map((proposal) => (
        <div
          key={proposal.id}
          className={
            proposal.id === selectedId ? "run-row selected" : "run-row"
          }
          onClick={() => {
            onSelect(proposal.id);
          }}
        >
          {proposal.name}{" "}
          <span className={`kind-badge kind-${proposal.kind}`}>
            {proposal.kind}
          </span>
          <br />
          <small>
            {proposal.source_flow} —{" "}
            {new Date(proposal.created_at).toLocaleString()}
            {proposal.status === "applying" ? " — applying" : ""}
          </small>
          {proposal.error !== null && (
            <p className="error-text">{proposal.error}</p>
          )}
        </div>
      ))}
    </div>
  );
};

export const ProposalList = ({
  pending,
  failed,
  inProgress,
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
      {busy ? "Working…" : "Refresh"}
    </button>
    {error !== null && <p className="error-text">{error}</p>}
    {status !== null && <p className="status-text">{status}</p>}
    {pending.length + failed.length + inProgress.length === 0 && !busy && (
      <p>No proposals waiting.</p>
    )}
    <Section
      title="Pending"
      proposals={pending}
      selectedId={selectedId}
      onSelect={onSelect}
    />
    <Section
      title="Failed"
      proposals={failed}
      selectedId={selectedId}
      onSelect={onSelect}
    />
    <Section
      title="In progress"
      proposals={inProgress}
      selectedId={selectedId}
      onSelect={onSelect}
    />
  </div>
);
