import { backend } from "./wmill";

export const listProposals = (): Promise<unknown> => backend.list_proposals({});

export const getProposal = (proposalId: string): Promise<unknown> =>
  backend.get_proposal({ proposal_id: proposalId });

export const decideProposal = (
  proposalId: string,
  decision: "approve" | "reject",
  reason: string,
): Promise<unknown> =>
  backend.decide_proposal({
    proposal_id: proposalId,
    decision,
    reason,
  });

export const retryProposal = (proposalId: string): Promise<unknown> =>
  backend.retry_proposal({ proposal_id: proposalId });

export const chatProposal = (
  proposalId: string,
  message: string,
  historyJson: string,
  model: string,
): Promise<unknown> =>
  backend.chat_proposal({
    proposal_id: proposalId,
    message,
    history_json: historyJson,
    model,
  });

export const applyRevision = (
  proposalId: string,
  reply: string,
): Promise<unknown> =>
  backend.apply_revision({ proposal_id: proposalId, reply });
