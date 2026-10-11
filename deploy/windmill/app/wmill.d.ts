export declare const backend: {
  list_proposals: (args: Record<string, never>) => Promise<unknown>;
  get_proposal: (args: { proposal_id: string }) => Promise<unknown>;
  decide_proposal: (args: {
    proposal_id: string;
    decision: string;
    reason?: string;
  }) => Promise<unknown>;
  retry_proposal: (args: { proposal_id: string }) => Promise<unknown>;
  chat_proposal: (args: {
    proposal_id: string;
    message: string;
    history_json?: string;
    model?: string;
  }) => Promise<unknown>;
  apply_revision: (args: {
    proposal_id: string;
    reply: string;
  }) => Promise<unknown>;
};
