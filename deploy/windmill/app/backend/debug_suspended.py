import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import TypeAlias, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"


def api_base() -> str:
    return os.environ.get("BASE_INTERNAL_URL", "http://windmill-server:8000") + "/api"


def api_get(path: str) -> Json:
    request = urllib.request.Request(
        api_base() + path,
        headers={"Authorization": "Bearer " + os.environ["WM_TOKEN"]},
    )
    try:
        with urllib.request.urlopen(request) as response:
            return cast(Json, json.load(response))
    except urllib.error.HTTPError as error:
        return f"HTTP {error.code}: {error.read().decode()[:300]}"


def main() -> dict[str, Json]:
    workspace = os.environ["WM_WORKSPACE"]
    out: dict[str, Json] = {"workspace": workspace, "base": api_base()}

    plain = api_get(f"/w/{workspace}/jobs/queue/list")
    out["queue_all_count"] = len(plain) if isinstance(plain, list) else plain
    if isinstance(plain, list):
        out["queue_all_summary"] = [
            {
                "id": job.get("id"),
                "script_path": job.get("script_path"),
                "suspend": job.get("suspend"),
                "running": job.get("running"),
                "job_kind": job.get("job_kind"),
            }
            for job in plain[:10]
            if isinstance(job, dict)
        ]

    query = urllib.parse.urlencode(
        {"script_path_exact": "f/memory/taskmine", "suspended": "true"}
    )
    filtered = api_get(f"/w/{workspace}/jobs/queue/list?{query}")
    out["taskmine_suspended"] = filtered

    if isinstance(filtered, list) and filtered:
        first = filtered[0]
        if isinstance(first, dict):
            job_id = first.get("id")
            full = api_get(f"/w/{workspace}/jobs_u/get/{job_id}?no_logs=true&no_code=true")
            if isinstance(full, dict):
                out["full_job_keys"] = sorted(full.keys())
                out["flow_status"] = full.get("flow_status")
            else:
                out["full_job"] = full
    return out
