import json
import os
import urllib.parse
import urllib.request
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"


class SuspendedRun(TypedDict):
    job_id: str
    flow: str
    started_at: str
    markdown: str


FLOW_PATHS = ("f/memory/taskmine", "f/memory/daily_synth")


def api_base() -> str:
    return os.environ.get("BASE_INTERNAL_URL", "http://windmill-server:8000") + "/api"


def api_get(path: str) -> Json:
    request = urllib.request.Request(
        api_base() + path,
        headers={"Authorization": "Bearer " + os.environ["WM_TOKEN"]},
    )
    with urllib.request.urlopen(request) as response:
        return cast(Json, json.load(response))


def as_dict(value: Json) -> dict[str, Json]:
    return value if isinstance(value, dict) else {}


def as_str(value: Json) -> str:
    return value if isinstance(value, str) else ""


def suspended_step_job(job: dict[str, Json]) -> str:
    flow_status = as_dict(job.get("flow_status"))
    modules = flow_status.get("modules")
    if not isinstance(modules, list):
        return ""
    for module_json in modules:
        module = as_dict(module_json)
        if module.get("type") == "WaitingForEvents":
            return as_str(module.get("job"))
    return ""


def step_markdown(workspace: str, step_job_id: str) -> str:
    if not step_job_id:
        return ""
    result = api_get(f"/w/{workspace}/jobs_u/completed/get_result/{step_job_id}")
    if isinstance(result, dict):
        return as_str(result.get("markdown"))
    return str(result)


def main() -> list[SuspendedRun]:
    workspace = os.environ["WM_WORKSPACE"]
    rows: list[SuspendedRun] = []
    for flow in FLOW_PATHS:
        query = urllib.parse.urlencode(
            {"script_path_exact": flow, "suspended": "true", "running": "true"}
        )
        jobs = api_get(f"/w/{workspace}/jobs/queue/list?{query}")
        if not isinstance(jobs, list):
            continue
        for job_json in jobs:
            job = as_dict(job_json)
            step_job_id = suspended_step_job(job)
            if not step_job_id:
                continue
            started = as_str(job.get("started_at")) or as_str(job.get("created_at"))
            rows.append(
                SuspendedRun(
                    job_id=as_str(job.get("id")),
                    flow=flow,
                    started_at=started,
                    markdown=step_markdown(workspace, step_job_id),
                )
            )
    return rows
