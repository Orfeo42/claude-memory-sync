import json
import os
import urllib.request
from typing import cast


def api_base() -> str:
    return os.environ.get("BASE_INTERNAL_URL", "http://windmill-server:8000") + "/api"


def api_post(path: str, body: dict[str, str]) -> str:
    request = urllib.request.Request(
        api_base() + path,
        data=json.dumps(body).encode(),
        headers={
            "Authorization": "Bearer " + os.environ["WM_TOKEN"],
            "Content-Type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(request) as response:
        return cast(bytes, response.read()).decode()


def main(job_id: str, reason: str = "rejected from approval inbox") -> str:
    workspace = os.environ["WM_WORKSPACE"]
    api_post(f"/w/{workspace}/jobs_u/queue/cancel/{job_id}", {"reason": reason})
    return f"canceled {job_id}"
