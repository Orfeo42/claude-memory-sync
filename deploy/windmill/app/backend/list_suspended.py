import json
import os
import re
import urllib.request
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"


class ProposalItem(TypedDict):
    run_id: str
    flow: str
    started_at: str
    group: str
    content: str


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


def flow_display_name(script_path: str) -> str:
    parts = script_path.split("/")
    if parts and parts[-1].startswith("branchone"):
        parts = parts[:-1]
    return parts[-1] if parts else script_path


def find_waiting_step_job(value: Json) -> str:
    if isinstance(value, dict):
        if value.get("type") == "WaitingForEvents":
            step_job = as_str(value.get("job"))
            if step_job:
                return step_job
        for child in value.values():
            found = find_waiting_step_job(child)
            if found:
                return found
    if isinstance(value, list):
        for child in value:
            found = find_waiting_step_job(child)
            if found:
                return found
    return ""


def split_frontmatter(content: str) -> tuple[str, str]:
    if not content.startswith("---\n"):
        return "", content
    end = content.find("\n---\n", 4)
    if end == -1:
        return "", content
    return content[4:end], content[end + 5 :]


def group_content(files: Json) -> str:
    if not isinstance(files, list):
        return ""
    parts: list[str] = []
    for file_json in files:
        entry = as_dict(file_json)
        target = as_str(entry.get("target"))
        content = as_str(entry.get("content"))
        parts.append(f"**`{target}`**\n")
        if target.endswith(".md"):
            frontmatter, body = split_frontmatter(content)
            if frontmatter:
                parts.append("```yaml\n" + frontmatter + "\n```\n")
            parts.append(body)
        else:
            parts.append("```\n" + content + "\n```\n")
    return "\n".join(parts)


def items_from_markdown(markdown: str) -> list[tuple[str, str]]:
    merged: dict[str, list[str]] = {}
    current: str | None = None
    fence: str | None = None
    for line in markdown.splitlines():
        stripped = line.strip()
        fence_match = re.match(r"^(`{3,})", stripped)
        if fence_match:
            marker = fence_match.group(1)
            if fence is None:
                fence = marker
            elif len(marker) >= len(fence):
                fence = None
        elif fence is None and line.startswith("## "):
            current = line[3:].strip()
            merged.setdefault(current, [])
            continue
        if current is not None:
            merged[current].append(line)
    return [
        (name, unwrap_markdown_fences("\n".join(lines).strip()))
        for name, lines in merged.items()
    ]


def unwrap_markdown_fences(body: str) -> str:
    return re.sub(r"^````markdown\n(.*?)\n````", r"\1", body, flags=re.S | re.M)


def step_items(workspace: str, step_job_id: str) -> list[tuple[str, str]]:
    result = api_get(f"/w/{workspace}/jobs_u/completed/get_result/{step_job_id}")
    payload = as_dict(result)
    groups = payload.get("groups")
    if isinstance(groups, list):
        items: list[tuple[str, str]] = []
        for group_json in groups:
            group = as_dict(group_json)
            items.append((as_str(group.get("name")), group_content(group.get("files"))))
        return items
    return items_from_markdown(as_str(payload.get("markdown")))


def main() -> list[ProposalItem]:
    workspace = os.environ["WM_WORKSPACE"]
    rows: list[ProposalItem] = []
    jobs = api_get(f"/w/{workspace}/jobs/queue/list?suspended=true")
    if not isinstance(jobs, list):
        return rows
    for job_json in jobs:
        listed = as_dict(job_json)
        job_id = as_str(listed.get("id"))
        if not job_id:
            continue
        full = as_dict(api_get(f"/w/{workspace}/jobs_u/get/{job_id}?no_logs=true&no_code=true"))
        step_job_id = find_waiting_step_job(full.get("flow_status"))
        flow = flow_display_name(as_str(listed.get("script_path")))
        started = as_str(listed.get("started_at")) or as_str(listed.get("created_at"))
        items = step_items(workspace, step_job_id) if step_job_id else []
        if not items:
            items = [("(payload not located)", "_Approval step payload could not be located; use the run page._")]
        for group, content in items:
            rows.append(
                ProposalItem(
                    run_id=job_id,
                    flow=flow,
                    started_at=started,
                    group=group,
                    content=content,
                )
            )
    return rows
