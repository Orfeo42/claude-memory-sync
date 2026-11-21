import type { JSX } from "react";
import { formatTime } from "./format";
import type { Proposal } from "./types";

type Props = {
  pending: readonly Proposal[];
  failed: readonly Proposal[];
  inProgress: readonly Proposal[];
  selectedId: string | null;
  loading: boolean;
  busy: boolean;
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
    <section className="group" aria-label={title}>
      <div className="group-header">
        <strong>{title}</strong>
        <small>{proposals.length}</small>
      </div>
      {proposals.map((proposal) => (
        <button
          type="button"
          key={proposal.id}
          className={
            proposal.id === selectedId ? "row-button selected" : "row-button"
          }
          onClick={() => {
            onSelect(proposal.id);
          }}
        >
          <span>
            {proposal.name}{" "}
            <span className={`badge kind-${proposal.kind}`}>
              {proposal.kind}
            </span>
          </span>
          <small>
            {proposal.source_flow} — {formatTime(proposal.created_at)}
            {proposal.status === "applying" ? " — applying" : ""}
          </small>
          {proposal.error !== null && proposal.error !== undefined && (
            <span className="error-text">{proposal.error}</span>
          )}
        </button>
      ))}
    </section>
  );
};

export const ProposalList = ({
  pending,
  failed,
  inProgress,
  selectedId,
  loading,
  busy,
  onSelect,
  onRefresh,
}: Props): JSX.Element => (
  <div className="list-pane">
    <div className="toolbar">
      <h2>Approval inbox</h2>
      <button type="button" onClick={onRefresh} disabled={busy}>
        {busy ? "Working…" : "Refresh"}
      </button>
    </div>
    {loading && <p className="hint">Loading…</p>}
    {!loading &&
      pending.length + failed.length + inProgress.length === 0 && (
        <p className="hint">No proposals waiting.</p>
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
