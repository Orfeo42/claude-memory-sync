import * as React from "react";
import type { JSX } from "react";
import ReactMarkdown from "react-markdown";
import type { Decision, ProposalItem } from "./types";

type Props = {
  item: ProposalItem | null;
  decision: Decision | undefined;
  busy: boolean;
  onDecide: (decision: Decision) => void;
};

export const RunDetail = ({
  item,
  decision,
  busy,
  onDecide,
}: Props): JSX.Element => {
  if (item === null) {
    return (
      <div className="detail">
        <p>Select a proposal to review it.</p>
      </div>
    );
  }

  return (
    <div className="detail">
      <div className="detail-header">
        <h3>{item.group}</h3>
        <small>
          {item.flow} — {new Date(item.started_at).toLocaleString()}
        </small>
      </div>
      <div className="markdown">
        <ReactMarkdown>{item.content}</ReactMarkdown>
      </div>
      <div className="actions">
        <button
          type="button"
          className={decision === "approved" ? "primary" : ""}
          onClick={() => {
            onDecide("approved");
          }}
          disabled={busy}
        >
          {decision === "approved" ? "✓ Approved" : "Approve"}
        </button>
        <button
          type="button"
          className="danger"
          onClick={() => {
            onDecide("rejected");
          }}
          disabled={busy}
        >
          {decision === "rejected" ? "✗ Rejected" : "Reject"}
        </button>
      </div>
    </div>
  );
};
