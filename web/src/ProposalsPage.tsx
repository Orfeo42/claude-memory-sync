import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import {
  applyRevision,
  chatProposal,
  decideProposal,
  errorMessage,
  getProposal,
  listProposals,
  retryProposal,
} from "./api";
import { ProposalDetail } from "./ProposalDetail";
import { ProposalList } from "./ProposalList";
import type {
  ChatMessage,
  Model,
  Proposal,
  ProposalDetail as Detail,
} from "./types";

const withApplyNote = (verb: string, applyStarted: boolean): string =>
  applyStarted ? `Proposal ${verb}, apply started` : `Proposal ${verb}`;

export const ProposalsPage = (): JSX.Element => {
  const [proposals, setProposals] = useState<readonly Proposal[]>([]);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [chats, setChats] = useState<
    Readonly<Record<string, readonly ChatMessage[]>>
  >({});
  const [model, setModel] = useState<Model>("sonnet");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [chatBusy, setChatBusy] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const nextMessageId = useRef(1);
  const selectedRef = useRef<string | null>(null);

  const load = useCallback(async (signal?: AbortSignal): Promise<void> => {
    try {
      const list = await listProposals(signal);
      setProposals(list);
      setError(null);
    } catch (loadError: unknown) {
      if (signal?.aborted === true) {
        return;
      }
      setError(errorMessage(loadError));
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const start = async (): Promise<void> => {
      await load(controller.signal);
    };
    void start();
    return () => {
      controller.abort();
    };
  }, [load]);

  const refresh = useCallback(async (): Promise<void> => {
    setBusy(true);
    await load();
    setBusy(false);
  }, [load]);

  const select = useCallback(async (id: string): Promise<void> => {
    selectedRef.current = id;
    setSelectedId(id);
    setDetail(null);
    setError(null);
    try {
      const result = await getProposal(id);
      if (selectedRef.current === id) {
        setDetail(result);
      }
    } catch (selectError: unknown) {
      setError(errorMessage(selectError));
    }
  }, []);

  const finishAction = useCallback(
    async (message: string): Promise<void> => {
      selectedRef.current = null;
      setSelectedId(null);
      setDetail(null);
      await load();
      setStatus(message);
    },
    [load],
  );

  const decide = useCallback(
    async (
      id: string,
      decision: "approve" | "reject",
      reason: string,
    ): Promise<void> => {
      setBusy(true);
      setError(null);
      try {
        const result = await decideProposal(id, decision, reason);
        await finishAction(
          withApplyNote(
            decision === "approve" ? "approved" : "rejected",
            result.apply_started,
          ),
        );
      } catch (decideError: unknown) {
        setError(errorMessage(decideError));
      }
      setBusy(false);
    },
    [finishAction],
  );

  const retry = useCallback(
    async (id: string): Promise<void> => {
      setBusy(true);
      setError(null);
      try {
        const result = await retryProposal(id);
        await finishAction(withApplyNote("retried", result.apply_started));
      } catch (retryError: unknown) {
        setError(errorMessage(retryError));
      }
      setBusy(false);
    },
    [finishAction],
  );

  const send = useCallback(
    async (message: string): Promise<void> => {
      if (selectedId === null) {
        return;
      }
      const history = chats[selectedId] ?? [];
      const userId = nextMessageId.current++;
      setChats((current) => ({
        ...current,
        [selectedId]: [
          ...history,
          { id: userId, role: "user", content: message },
        ],
      }));
      setChatBusy(true);
      setError(null);
      try {
        const reply = await chatProposal(
          selectedId,
          message,
          model,
          history.map(({ role, content }) => ({ role, content })),
        );
        const replyId = nextMessageId.current++;
        setChats((current) => ({
          ...current,
          [selectedId]: [
            ...(current[selectedId] ?? []),
            { id: replyId, role: "assistant", content: reply },
          ],
        }));
      } catch (chatError: unknown) {
        setError(errorMessage(chatError));
      }
      setChatBusy(false);
    },
    [selectedId, chats, model],
  );

  const apply = useCallback(async (): Promise<void> => {
    if (selectedId === null) {
      return;
    }
    const lastAssistant = [...(chats[selectedId] ?? [])]
      .reverse()
      .find((m) => m.role === "assistant");
    if (lastAssistant === undefined) {
      return;
    }
    setChatBusy(true);
    setError(null);
    try {
      const result = await applyRevision(selectedId, lastAssistant.content);
      const revised = result.proposal;
      setDetail((current) =>
        current === null ? current : { ...current, proposal: revised },
      );
      setProposals((current) =>
        current.map((p) => (p.id === revised.id ? revised : p)),
      );
      setStatus(`Revision applied to ${String(result.applied.length)} file(s)`);
    } catch (applyError: unknown) {
      setError(errorMessage(applyError));
    }
    setChatBusy(false);
  }, [selectedId, chats]);

  const pending = proposals.filter((p) => p.status === "pending");
  const failed = proposals.filter((p) => p.status === "failed");
  const inProgress = proposals.filter(
    (p) => p.status === "approved" || p.status === "applying",
  );
  const selectedChat = selectedId === null ? [] : (chats[selectedId] ?? []);

  const onSelect = useCallback(
    (id: string): void => {
      void select(id);
    },
    [select],
  );
  const onRefresh = useCallback((): void => {
    void refresh();
  }, [refresh]);
  const onApprove = useCallback(
    (reason: string): void => {
      if (selectedId !== null) {
        void decide(selectedId, "approve", reason);
      }
    },
    [selectedId, decide],
  );
  const onReject = useCallback(
    (reason: string): void => {
      if (selectedId !== null) {
        void decide(selectedId, "reject", reason);
      }
    },
    [selectedId, decide],
  );
  const onRetry = useCallback((): void => {
    if (selectedId !== null) {
      void retry(selectedId);
    }
  }, [selectedId, retry]);
  const onSend = useCallback(
    (message: string): void => {
      void send(message);
    },
    [send],
  );
  const onApplyRevision = useCallback((): void => {
    void apply();
  }, [apply]);

  return (
    <div className="split">
      <div className="split-list">
        {error !== null && (
          <p className="error-text" role="alert">
            {error}
          </p>
        )}
        {status !== null && (
          <p className="status-text" role="status">
            {status}
          </p>
        )}
        <ProposalList
          pending={pending}
          failed={failed}
          inProgress={inProgress}
          selectedId={selectedId}
          loading={loading}
          busy={busy}
          onSelect={onSelect}
          onRefresh={onRefresh}
        />
      </div>
      <ProposalDetail
        key={selectedId ?? "none"}
        detail={detail}
        busy={busy}
        chat={selectedChat}
        chatBusy={chatBusy}
        model={model}
        onApprove={onApprove}
        onReject={onReject}
        onRetry={onRetry}
        onModelChange={setModel}
        onSend={onSend}
        onApplyRevision={onApplyRevision}
      />
    </div>
  );
};
