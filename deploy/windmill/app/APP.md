# Approval inbox — full-code Windmill App

A full-code (React) Windmill App that lists every suspended
(approval-pending) run of `f/memory/taskmine` and `f/memory/daily_synth`,
renders the staged proposals as markdown, and lets you resume with a
selective `approved` value or cancel the run.

Files in this directory, mirroring the in-browser editor's scaffold:

| File                        | Role                                                    |
| --------------------------- | ------------------------------------------------------- |
| `index.tsx`                 | Entry point (createRoot, imports `index.css`).          |
| `index.css`                 | All styles (layout, run rows, markdown panel, actions). |
| `App.tsx`                   | State + backend calls, composes the two components.     |
| `RunList.tsx`               | Left pane: pending runs, refresh, error/status lines.   |
| `RunDetail.tsx`             | Right pane: rendered markdown, approved input, buttons. |
| `types.ts`                  | `SuspendedRun` type + runtime guard + error helper.     |
| `package.json`              | Frontend deps (react 19, react-dom, react-markdown).    |
| `backend/list_suspended.py` | Runnable: suspended runs + their markdown payloads.     |
| `backend/resume_run.py`     | Runnable: resume a run with `{approved: ...}`.          |
| `backend/cancel_run.py`     | Runnable: cancel a run (discard the batch).             |

Not in this directory because the editor generates them: `raw_app.yaml`,
`wmill.d.ts` (regenerated from the backend runnables), and the `wmill`
runtime module the frontend imports `backend` from.

The backend scripts talk to the Windmill API through `BASE_INTERNAL_URL`

- `WM_TOKEN` (injected into every job by Windmill) — no extra secret.

## Setup — entirely in the Windmill UI

1. Home page → **New** → **App (full-code)**. Framework: **React 19**.
   Data: **none** (no datatable). Start **without AI**. Save the app as
   `f/memory/approval_inbox`.
2. In the runnables panel on the right, click **+** three times,
   language **Python**, and create the runnables with these exact names
   (the frontend calls them by name — the generated `wmill.d.ts` must
   end up declaring `list_suspended`, `resume_run`, `cancel_run`):
   - `list_suspended` — paste `backend/list_suspended.py`
   - `resume_run` — paste `backend/resume_run.py`
   - `cancel_run` — paste `backend/cancel_run.py`
3. Open `package.json` in the editor and add to `dependencies`:
   `"react-markdown": "^9.0.0"` (reference: this directory's
   `package.json`; keep whatever else the scaffold put there).
4. Mirror the frontend files: replace the scaffold's `App.tsx`,
   `index.tsx`, `index.css` with this directory's versions, and create
   `RunList.tsx`, `RunDetail.tsx`, `types.ts` as new files next to
   `App.tsx`. The scaffold's `index.tsx` import style doesn't matter —
   `App.tsx` exports both named and default.

## Test / deploy (still in the editor)

- Select `App.tsx` for the live preview pane, or click **Preview** for
  fullscreen. Backend runnables execute on the real worker even in
  preview.
- Click **Deploy** in the toolbar — the app appears under **Apps** as
  `f/memory/approval_inbox`.

## TODO verify against the installed Windmill version

The backend scripts were written against the documented API shape, not
validated live yet. If a runnable errors, check these first (API docs
served by your own instance at `http://<pi-host>:8081/openapi.html`):

- `GET /w/{ws}/jobs/queue/list` query params `suspended` and
  `script_path_exact`.
- Suspended step detection: `flow_status.modules[].type ==
"WaitingForEvents"` and its `job` field holding the step's job id.
- `GET /w/{ws}/jobs_u/completed/get_result/{id}` for the approval
  step's `{"markdown": ...}` result.
- `POST /w/{ws}/jobs/flow/resume/{id}` (owner resume, body = resume
  payload) and `POST /w/{ws}/jobs_u/queue/cancel/{id}` (body
  `{"reason": ...}`).
- The generated `wmill` module's exact import path/shape (`import
{ backend } from './wmill'`) — align `App.tsx`'s import with whatever
  the scaffold generates.

## Approval workflow once deployed

Open the App, click a run, read the rendered proposals, type the group
names you want in `approved` (or leave `all` / type `none`), hit
**Resume**. Rejected groups are deleted from staging by the flow itself
and remembered in the do-not-re-propose list — nothing to clean up
manually. **Cancel run** discards the whole batch instead.
