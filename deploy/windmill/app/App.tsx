import * as React from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { JSX } from "react";
import {
  applyRevision,
  chatProposal,
  decideProposal,
  getProposal,
  listProposals,
  retryProposal,
} from "./backendApi";
import { ProposalDetail } from "./ProposalDetail";
import { ProposalList } from "./ProposalList";
import {
  errorMessage,
  isDecideResult,
  isProposal,
  isProposalDetail,
  isReplyPayload,
  isRevisionPayload,
} from "./types";
import type {
  ChatMessage,
  Model,
  Proposal,
  ProposalDetail as Detail,
} from "./types";

const applyStatus = (applyJobId: string | null, verb: string): string =>
  applyJobId === null
    ? `Proposal ${verb}`
    : `Proposal ${verb}, apply job ${applyJobId} started`;

export const App = (): JSX.Element => {
  const [proposals, setProposals] = useState<readonly Proposal[]>([]);
  const [detail, setDetail] = useState<Detail | null>(null);
  const [chats, setChats] = useState<
    Readonly<Record<string, readonly ChatMessage[]>>
  >({});
  const [model, setModel] = useState<Model>("sonnet");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [chatBusy, setChatBusy] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const nextMessageId = useRef(1);

  const refresh = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const result: unknown = await listProposals();
      setProposals(Array.isArray(result) ? result.filter(isProposal) : []);
    } catch (loadError: unknown) {
      setError(errorMessage(loadError));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const select = useCallback(async (id: string): Promise<void> => {
    setSelectedId(id);
    setDetail(null);
    setError(null);
    try {
      const result: unknown = await getProposal(id);
      if (!isProposalDetail(result)) {
        throw new Error("unexpected proposal payload");
      }
      setDetail(result);
    } catch (selectError: unknown) {
      setError(errorMessage(selectError));
    }
  }, []);

  const finishDecision = useCallback(
    async (message: string): Promise<void> => {
      setSelectedId(null);
      setDetail(null);
      await refresh();
      setStatus(message);
    },
    [refresh],
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
        const result: unknown = await decideProposal(id, decision, reason);
        if (!isDecideResult(result)) {
          throw new Error("unexpected decision payload");
        }
        await finishDecision(
          applyStatus(
            result.apply_job_id ?? null,
            decision === "approve" ? "approved" : "rejected",
          ),
        );
      } catch (decideError: unknown) {
        setError(errorMessage(decideError));
        setBusy(false);
      }
    },
    [finishDecision],
  );

  const retry = useCallback(
    async (id: string): Promise<void> => {
      setBusy(true);
      setError(null);
      try {
        const result: unknown = await retryProposal(id);
        if (!isDecideResult(result)) {
          throw new Error("unexpected retry payload");
        }
        await finishDecision(
          applyStatus(result.apply_job_id ?? null, "retried"),
        );
      } catch (retryError: unknown) {
        setError(errorMessage(retryError));
        setBusy(false);
      }
    },
    [finishDecision],
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
        const result: unknown = await chatProposal(
          selectedId,
          message,
          JSON.stringify(
            history.map(({ role, content }) => ({ role, content })),
          ),
          model,
        );
        if (!isReplyPayload(result)) {
          throw new Error("unexpected chat reply payload");
        }
        const replyId = nextMessageId.current++;
        setChats((current) => ({
          ...current,
          [selectedId]: [
            ...(current[selectedId] ?? []),
            { id: replyId, role: "assistant", content: result.reply },
          ],
        }));
      } catch (chatError: unknown) {
        setError(errorMessage(chatError));
      } finally {
        setChatBusy(false);
      }
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
      const result: unknown = await applyRevision(
        selectedId,
        lastAssistant.content,
      );
      if (!isRevisionPayload(result)) {
        throw new Error("unexpected revision payload");
      }
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
    } finally {
      setChatBusy(false);
    }
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
    <div className="page">
      <ProposalList
        pending={pending}
        failed={failed}
        inProgress={inProgress}
        selectedId={selectedId}
        busy={busy}
        error={error}
        status={status}
        onSelect={onSelect}
        onRefresh={onRefresh}
      />
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

export default App;
