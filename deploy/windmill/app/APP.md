# Approval inbox App — assembly recipe

A Windmill App that lists every suspended (approval-pending) run of
`f/memory/taskmine` and `f/memory/daily_synth`, renders the staged
proposals as markdown, and lets you resume with a selective `approved`
value or cancel the run — no more digging through the Runs page.

## Backend scripts

Save each `*.py` in this directory as a workspace script first
(**Scripts** → **New script** → language Python, paste content):

| Script path suggestion          | File                | Purpose                                                            |
| ------------------------------- | ------------------- | ------------------------------------------------------------------ |
| `f/memory/app_list_suspended`   | `list_suspended.py` | List suspended runs of both approval flows + their markdown payload. |
| `f/memory/app_resume_run`       | `resume_run.py`     | Resume a run with `{approved: ...}`.                               |
| `f/memory/app_cancel_run`       | `cancel_run.py`     | Cancel a run (discard the whole batch).                            |

The scripts talk to the Windmill API directly through
`BASE_INTERNAL_URL` + `WM_TOKEN` (both injected into every job by
Windmill itself) — no extra secret or resource needed.

### TODO verify against the installed Windmill version

Written against the documented API shape, not validated live yet. If a
script errors, check these first (the API docs are served by your own
instance at `http://<host>:8081/openapi.html`):

- `GET /w/{ws}/jobs/queue/list` query params `suspended` and
  `script_path_exact`.
- Suspended step detection: `flow_status.modules[].type ==
  "WaitingForEvents"` and its `job` field holding the step's job id.
- `GET /w/{ws}/jobs_u/completed/get_result/{id}` for the approval
  step's `{"markdown": ...}` result.
- `POST /w/{ws}/jobs/flow/resume/{id}` (owner resume, body = resume
  payload) and `POST /w/{ws}/jobs_u/queue/cancel/{id}` (body
  `{"reason": ...}`).

## App assembly (UI)

**Apps** → **New app**, name it e.g. `f/memory/approval_inbox`, then:

1. **Table** component (top of the canvas):
   - Data source → **Select a script** → `f/memory/app_list_suspended`,
     no arguments.
   - Show columns `flow` and `started_at`; hide `markdown` and
     `job_id` (they stay available on the selected row).
2. **Markdown** component (below the table, make it tall):
   - Content → connect to `<table-id>.selectedRow.markdown` (eval
     input, e.g. `${a.selectedRow.markdown}` where `a` is the table's
     component id).
3. **Text input** component, id it `approved_input`:
   - Default value `all`. Label: "approved — all | none |
     comma-separated group names".
4. **Button** "Resume":
   - On click → run script `f/memory/app_resume_run` with
     `job_id = <table-id>.selectedRow.job_id`,
     `approved = approved_input.result` (connect both arguments to the
     component outputs).
   - After run: add the table's runnable to the button's
     **Recompute others** list so the inbox refreshes.
5. **Button** "Cancel run" (danger color):
   - On click → `f/memory/app_cancel_run` with
     `job_id = <table-id>.selectedRow.job_id`.
   - Same recompute-table-after setting.

Approval workflow from here on: open the App, click a row, read the
rendered proposals, type the group names you want (or leave `all` /
type `none`), hit Resume. Rejected groups are deleted from staging by
the flow itself — nothing to clean up manually.
