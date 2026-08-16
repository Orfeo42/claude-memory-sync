import * as React from "react";
import { useState } from "react";
import type { JSX } from "react";
import ReactMarkdown from "react-markdown";
import type { ChatMessage, Decision, Model, ProposalItem } from "./types";
import { MODELS } from "./types";

type Props = {
  item: ProposalItem | null;
  decision: Decision | undefined;
  busy: boolean;
  chat: readonly ChatMessage[];
  chatBusy: boolean;
  model: Model;
  onDecide: (decision: Decision) => void;
  onModelChange: (model: Model) => void;
  onSend: (message: string) => void;
  onApplyRevision: () => void;
};

const isModel = (value: string): value is Model =>
  (MODELS as readonly string[]).includes(value);

export const RunDetail = ({
  item,
  decision,
  busy,
  chat,
  chatBusy,
  model,
  onDecide,
  onModelChange,
  onSend,
  onApplyRevision,
}: Props): JSX.Element => {
  const [draft, setDraft] = useState("");

  if (item === null) {
    return (
      <div className="detail">
        <p>Select a proposal to review it.</p>
      </div>
    );
  }

  const lastAssistant = [...chat].reverse().find((m) => m.role === "assistant");
  const canApply =
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
    <div className="detail">
      <div className="detail-header">
        <h3>
          {item.group}{" "}
          <span className={`kind-badge kind-${item.kind}`}>{item.kind}</span>
        </h3>
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
      <div className="chat">
        <div className="chat-messages">
          {chat.length === 0 && (
            <p className="chat-hint">
              Discuss this proposal with the model — ask for changes, then apply
              its revision to the staged files before approving.
            </p>
          )}
          {chat.map((message, index) => (
            <div key={index} className={`chat-message chat-${message.role}`}>
              <ReactMarkdown>{message.content}</ReactMarkdown>
            </div>
          ))}
          {chatBusy && <p className="chat-hint">Thinking…</p>}
        </div>
        <div className="chat-input">
          <select
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
    </div>
  );
};
