import * as React from "react";
import { useCallback, useEffect, useState } from "react";
import type { JSX } from "react";
import {
  applyRevision,
  cancelRun,
  chatProposal,
  listSuspended,
  resumeRun,
} from "./backendApi";
import { RunDetail } from "./RunDetail";
import { RunList } from "./RunList";
import {
  errorMessage,
  groupByRun,
  isProposalItem,
  isReplyPayload,
  isRevisionPayload,
  itemKey,
} from "./types";
import type { ChatMessage, Decision, Model, ProposalItem } from "./types";

export const App = (): JSX.Element => {
  const [items, setItems] = useState<readonly ProposalItem[]>([]);
  const [decisions, setDecisions] = useState<
    Readonly<Record<string, Decision>>
  >({});
  const [chats, setChats] = useState<
    Readonly<Record<string, readonly ChatMessage[]>>
  >({});
  const [model, setModel] = useState<Model>("sonnet");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [chatBusy, setChatBusy] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const result: unknown = await listSuspended();
      const rows = Array.isArray(result) ? result.filter(isProposalItem) : [];
      setItems(rows);
      const validKeys = new Set(rows.map(itemKey));
      setSelectedKey((current) =>
        current !== null && validKeys.has(current) ? current : null,
      );
      setDecisions((current) => {
        const kept: Record<string, Decision> = {};
        for (const [key, decision] of Object.entries(current)) {
          if (validKeys.has(key)) {
            kept[key] = decision;
          }
        }
        return kept;
      });
    } catch (loadError: unknown) {
      setError(errorMessage(loadError));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const decide = useCallback(
    (decision: Decision): void => {
      if (selectedKey === null) {
        return;
      }
      setDecisions((current) => ({ ...current, [selectedKey]: decision }));
    },
    [selectedKey],
  );

  const submit = useCallback(
    async (runId: string): Promise<void> => {
      const runItems = items.filter((item) => item.run_id === runId);
      const approvedNames = runItems
        .filter((item) => decisions[itemKey(item)] === "approved")
        .map((item) => item.group);
      const approved =
        approvedNames.length > 0 ? approvedNames.join(",") : "none";
      setBusy(true);
      setError(null);
      try {
        await resumeRun(runId, approved);
        setStatus(`Submitted: approved=${approved}`);
        await refresh();
      } catch (submitError: unknown) {
        setError(errorMessage(submitError));
        setBusy(false);
      }
    },
    [items, decisions, refresh],
  );

  const cancel = useCallback(
    async (runId: string): Promise<void> => {
      if (!window.confirm("Cancel this run and discard its whole batch?")) {
        return;
      }
      setBusy(true);
      setError(null);
      try {
        await cancelRun(runId, "rejected from approval inbox");
        setStatus("Run canceled, batch discarded");
        await refresh();
      } catch (cancelError: unknown) {
        setError(errorMessage(cancelError));
        setBusy(false);
      }
    },
    [refresh],
  );

  const selected = items.find((item) => itemKey(item) === selectedKey) ?? null;
  const selectedChat = selectedKey === null ? [] : (chats[selectedKey] ?? []);

  const send = useCallback(
    async (message: string): Promise<void> => {
      if (selected === null || selectedKey === null) {
        return;
      }
      const history = chats[selectedKey] ?? [];
      setChats((current) => ({
        ...current,
        [selectedKey]: [...history, { role: "user", content: message }],
      }));
      setChatBusy(true);
      setError(null);
      try {
        const result: unknown = await chatProposal(
          selected.run_id,
          selected.group,
          message,
          JSON.stringify(history),
          model,
        );
        if (!isReplyPayload(result)) {
          throw new Error("unexpected chat reply payload");
        }
        setChats((current) => ({
          ...current,
          [selectedKey]: [
            ...(current[selectedKey] ?? []),
            { role: "assistant", content: result.reply },
          ],
        }));
      } catch (chatError: unknown) {
        setError(errorMessage(chatError));
      } finally {
        setChatBusy(false);
      }
    },
    [selected, selectedKey, chats, model],
  );

  const apply = useCallback(async (): Promise<void> => {
    if (selected === null || selectedKey === null) {
      return;
    }
    const lastAssistant = [...(chats[selectedKey] ?? [])]
      .reverse()
      .find((m) => m.role === "assistant");
    if (lastAssistant === undefined) {
      return;
    }
    setChatBusy(true);
    setError(null);
    try {
      const result: unknown = await applyRevision(
        selected.run_id,
        selected.group,
        lastAssistant.content,
      );
      if (!isRevisionPayload(result)) {
        throw new Error("unexpected revision payload");
      }
      setItems((current) =>
        current.map((item) =>
          itemKey(item) === selectedKey
            ? { ...item, content: result.content }
            : item,
        ),
      );
      setStatus(`Revision applied to ${String(result.applied.length)} file(s)`);
    } catch (applyError: unknown) {
      setError(errorMessage(applyError));
    } finally {
      setChatBusy(false);
    }
  }, [selected, selectedKey, chats]);

  return (
    <div className="page">
      <RunList
        runs={groupByRun(items)}
        decisions={decisions}
        selectedKey={selectedKey}
        busy={busy}
        error={error}
        status={status}
        onSelect={setSelectedKey}
        onRefresh={() => {
          void refresh();
        }}
        onSubmit={(runId) => {
          void submit(runId);
        }}
        onCancelRun={(runId) => {
          void cancel(runId);
        }}
      />
      <RunDetail
        key={selectedKey ?? "none"}
        item={selected}
        decision={selected === null ? undefined : decisions[itemKey(selected)]}
        busy={busy}
        chat={selectedChat}
        chatBusy={chatBusy}
        model={model}
        onDecide={decide}
        onModelChange={setModel}
        onSend={(message) => {
          void send(message);
        }}
        onApplyRevision={() => {
          void apply();
        }}
      />
    </div>
  );
};

export default App;
