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


def main(job_id: str, approved: str = "all") -> str:
    workspace = os.environ["WM_WORKSPACE"]
    api_post(f"/w/{workspace}/jobs/flow/resume/{job_id}", {"approved": approved})
    return f"resumed {job_id} with approved={approved}"
