# Approval inbox — full-code Windmill App

A full-code (React) Windmill App that lists the proposals held by `pipeline-api` (pending, failed, approved/applying), renders their files as markdown, and lets you approve, reject, retry, or refine a pending proposal through a chat with the model. It no longer touches suspended Windmill runs.

All proposal access goes through `memoryctl proposal …` (baked into the `ui-backend` worker image, which also has `PIPELINE_URL` and the admin `PIPELINE_TOKEN`). The Python runnables never talk to pipeline-api directly. After every approve and every retry the runnables start the apply flow through the Windmill API (`BASE_INTERNAL_URL` + `WM_TOKEN`, injected into every job), so there is no extra secret.

Files in this directory, mirroring the in-browser editor's scaffold:

| File                         | Role                                                                    |
| ---------------------------- | ----------------------------------------------------------------------- |
| `index.tsx`                  | Entry point (createRoot, imports `index.css`).                          |
| `index.css`                  | All styles (layout, rows, markdown panel, actions, chat).               |
| `App.tsx`                    | State + backend calls, composes the two components.                     |
| `ProposalList.tsx`           | Left pane: Pending / Failed / In progress sections, refresh, status.    |
| `ProposalDetail.tsx`         | Right pane: files, history, approve/reject/retry, chat + revision.      |
| `types.ts`                   | Proposal types, runtime guards, models, error helper.                   |
| `backendApi.ts`              | Typed wrappers over the generated `backend` runnables.                  |
| `theme.ts`                   | Syncs Windmill theme tokens into the iframe.                            |
| `package.json`               | Frontend deps (react 19, react-dom, react-markdown).                    |
| `backend/list_proposals.py`  | Runnable: pending + failed + approved + applying proposals.             |
| `backend/get_proposal.py`    | Runnable: one proposal with its event history.                          |
| `backend/decide_proposal.py` | Runnable: approve/reject; on approve starts the apply flow.             |
| `backend/retry_proposal.py`  | Runnable: failed -> approved, then starts the apply flow.               |
| `backend/chat_proposal.py`   | Runnable: `claude -p` chat about a proposal's files.                    |
| `backend/apply_revision.py`  | Runnable: parse `===FILE:` blocks from a reply and revise the proposal. |

Not in this directory because the editor generates them: `raw_app.yaml`, `wmill.d.ts` (regenerated from the backend runnables; a reference copy is kept here), and the `wmill` runtime module the frontend imports `backend` from.

Each runnable is self-contained (pasted separately into the editor), so the small helpers (`Json` alias, `run_memoryctl`, `to_proposal`) are duplicated on purpose.

## Apply-flow path

`decide_proposal.py` and `retry_proposal.py` define the module constant `APPLY_FLOW_PATH = "f/u/luciorosadini87/memory_proposal_apply"` and `POST {BASE_INTERNAL_URL}/api/w/{WM_WORKSPACE}/jobs/run/{APPLY_FLOW_PATH}` with body `{}`. Change the constant in both files if the flow moves.

## Setup — entirely in the Windmill UI

1. Home page -> **New** -> **App (full-code)**. Framework: **React 19**. Data: **none** (no datatable). Start **without AI**. Save the app as `f/memory/approval_inbox` (or open the existing one).
2. In the runnables panel on the right, delete the four old runnables (`list_suspended`, `resume_run`, `cancel_run`, `debug_suspended`), then click **+** six times, language **Python**, and create the runnables with these exact names (the frontend calls them by name — the generated `wmill.d.ts` must end up declaring all six):
   - `list_proposals` — paste `backend/list_proposals.py`
   - `get_proposal` — paste `backend/get_proposal.py`
   - `decide_proposal` — paste `backend/decide_proposal.py`
   - `retry_proposal` — paste `backend/retry_proposal.py`
   - `chat_proposal` — paste `backend/chat_proposal.py`
   - `apply_revision` — paste `backend/apply_revision.py`
3. Make sure every runnable runs on the `ui-backend` worker group (it has `memoryctl`, `PIPELINE_URL`, `PIPELINE_TOKEN`, and the `claude` CLI for the chat).
4. Open `package.json` in the editor and add to `dependencies`: `"react-markdown": "^9.0.0"` (reference: this directory's `package.json`; keep whatever else the scaffold put there).
5. Mirror the frontend files: replace the scaffold's `App.tsx`, `index.tsx`, `index.css` with this directory's versions, delete `RunList.tsx` / `RunDetail.tsx` if present, and create `ProposalList.tsx`, `ProposalDetail.tsx`, `types.ts`, `backendApi.ts`, `theme.ts` next to `App.tsx`. `App.tsx` exports both named and default.

## Test / deploy (still in the editor)

- Select `App.tsx` for the live preview pane, or click **Preview** for fullscreen. Backend runnables execute on the real worker even in preview.
- Click **Deploy** in the toolbar — the app appears under **Apps** as `f/memory/approval_inbox`.

## Approval workflow

Open the App and click a pending proposal. Read the rendered files and the history, optionally chat with the model and **Apply revision** to rewrite the proposal's files, then **Approve** or **Reject** (optional reason). Approve starts the apply flow and shows `apply job <id> started`; the proposal moves to In progress until the flow marks it applied. A failed proposal shows its error and a **Retry** button, which re-queues it and starts the apply flow again.

## Local checks

Inside this directory: `npx tsc --noEmit`, `npm run lint`, `uvx mypy --strict backend/*.py`, `uvx ruff check backend/`.
