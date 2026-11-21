import { useState } from "react";
import type { JSX } from "react";
import ReactMarkdown from "react-markdown";
import { formatTime } from "./format";
import { MODELS } from "./types";
import type { ChatMessage, Model, ProposalDetail as Detail } from "./types";

type Props = {
  detail: Detail | null;
  busy: boolean;
  chat: readonly ChatMessage[];
  chatBusy: boolean;
  model: Model;
  onApprove: (reason: string) => void;
  onReject: (reason: string) => void;
  onRetry: () => void;
  onModelChange: (model: Model) => void;
  onSend: (message: string) => void;
  onApplyRevision: () => void;
};

const isModel = (value: string): value is Model =>
  MODELS.some((model) => model === value);

export const ProposalDetail = ({
  detail,
  busy,
  chat,
  chatBusy,
  model,
  onApprove,
  onReject,
  onRetry,
  onModelChange,
  onSend,
  onApplyRevision,
}: Props): JSX.Element => {
  const [draft, setDraft] = useState("");
  const [reason, setReason] = useState("");

  if (detail === null) {
    return (
      <div className="detail-pane">
        <p className="hint">Select a proposal to review it.</p>
      </div>
    );
  }

  const { proposal, events } = detail;
  const files = proposal.files ?? [];
  const isPending = proposal.status === "pending";
  const lastAssistant = [...chat].reverse().find((m) => m.role === "assistant");
  const canApply =
    isPending &&
    !chatBusy &&
    lastAssistant !== undefined &&
    lastAssistant.content.includes("===FILE:");

  const send = (): void => {
    const message = draft.trim();
    if (message === "" || chatBusy) {
      return;
    }
    setDraft("");
    onSend(message);
  };

  return (
    <div className="detail-pane">
      <div className="detail-header">
        <h3>
          {proposal.name}{" "}
          <span className={`badge kind-${proposal.kind}`}>{proposal.kind}</span>{" "}
          <span className="badge">{proposal.status}</span>
        </h3>
        <small>
          {proposal.source_flow} — {formatTime(proposal.created_at)}
        </small>
        {proposal.error !== null && proposal.error !== undefined && (
          <p className="error-text">{proposal.error}</p>
        )}
      </div>
      <div className="markdown">
        {files.map((file) => (
          <div key={file.target_path}>
            <p>
              <strong>
                <code>{file.target_path}</code>
              </strong>
            </p>
            {file.target_path.endsWith(".md") ? (
              <ReactMarkdown>{file.content}</ReactMarkdown>
            ) : (
              <pre>
                <code>{file.content}</code>
              </pre>
            )}
          </div>
        ))}
        <details>
          <summary>History ({events.length})</summary>
          <ul className="events">
            {events.map((event) => (
              <li key={event.id}>
                <small>
                  {formatTime(event.at)} — {event.from_status ?? "∅"} →{" "}
                  {event.to_status} by {event.actor}
                  {event.note !== null &&
                  event.note !== undefined &&
                  event.note !== ""
                    ? ` — ${event.note}`
                    : ""}
                </small>
              </li>
            ))}
          </ul>
        </details>
      </div>
      {isPending && (
        <div className="actions">
          <input
            aria-label="Reason"
            value={reason}
            onChange={(event) => {
              setReason(event.target.value);
            }}
            placeholder="Reason (optional)"
            disabled={busy}
          />
          <button
            type="button"
            className="primary"
            onClick={() => {
              onApprove(reason.trim());
            }}
            disabled={busy}
          >
            Approve
          </button>
          <button
            type="button"
            className="danger"
            onClick={() => {
              onReject(reason.trim());
            }}
            disabled={busy}
          >
            Reject
          </button>
        </div>
      )}
      {proposal.status === "failed" && (
        <div className="actions">
          <button
            type="button"
            className="primary"
            onClick={onRetry}
            disabled={busy}
          >
            Retry
          </button>
        </div>
      )}
      {isPending && (
        <div className="chat">
          <div className="chat-messages">
            {chat.length === 0 && (
              <p className="hint">
                Discuss this proposal with the model — ask for changes, then
                apply its revision to the proposal files before approving.
              </p>
            )}
            {chat.map((message) => (
              <div
                key={message.id}
                className={`chat-message chat-${message.role}`}
              >
                <ReactMarkdown>{message.content}</ReactMarkdown>
              </div>
            ))}
            {chatBusy && <p className="hint">Thinking…</p>}
          </div>
          <div className="chat-input">
            <select
              aria-label="Model"
              value={model}
              onChange={(event) => {
                if (isModel(event.target.value)) {
                  onModelChange(event.target.value);
                }
              }}
              disabled={chatBusy}
            >
              {MODELS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
            <input
              aria-label="Message"
              value={draft}
              onChange={(event) => {
                setDraft(event.target.value);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  send();
                }
              }}
              placeholder="Ask for changes or clarifications…"
              disabled={chatBusy}
            />
            <button
              type="button"
              onClick={send}
              disabled={chatBusy || draft.trim() === ""}
            >
              Send
            </button>
            {canApply && (
              <button
                type="button"
                className="primary"
                onClick={onApplyRevision}
                disabled={busy}
              >
                Apply revision
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
