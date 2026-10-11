import json
import os
import re
import subprocess
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"

UUID = re.compile(r"[0-9a-f-]{36}")

FILE_BLOCK = re.compile(r"^===FILE: (?P<target>.+?)===\n(?P<content>.*?)\n===END===", re.DOTALL | re.MULTILINE)


class ProposalFile(TypedDict):
    target_path: str
    content: str


class Proposal(TypedDict):
    id: str
    source_flow: str
    source_run: str
    kind: str
    name: str
    status: str
    created_at: str
    updated_at: str
    decided_at: str | None
    decided_by: str | None
    claimed_by: str | None
    claimed_until: str | None
    applied_sha: str | None
    error: str | None
    files: list[ProposalFile]


class RevisionResult(TypedDict):
    applied: list[str]
    proposal: Proposal


def run_memoryctl(args: list[str], stdin: str | None = None) -> Json:
    result = subprocess.run(
        ["memoryctl", *args],
        input=stdin,
        capture_output=True,
        text=True,
        check=False,
        timeout=60,
    )
    if result.returncode != 0:
        raise RuntimeError(f"memoryctl {args[0]} {args[1]} failed: {result.stderr[-400:]}")
    return cast(Json, json.loads(result.stdout))


def as_dict(value: Json) -> dict[str, Json]:
    if not isinstance(value, dict):
        raise TypeError("expected a JSON object")
    return value


def as_list(value: Json) -> list[Json]:
    if value is None:
        return []
    if not isinstance(value, list):
        raise TypeError("expected a JSON array")
    return value


def as_str(data: dict[str, Json], key: str) -> str:
    value = data.get(key)
    if not isinstance(value, str):
        raise TypeError(f"expected string field {key}")
    return value


def opt_str(data: dict[str, Json], key: str) -> str | None:
    value = data.get(key)
    if value is None:
        return None
    if not isinstance(value, str):
        raise TypeError(f"expected string field {key}")
    return value


def to_proposal(value: Json) -> Proposal:
    data = as_dict(value)
    files: list[ProposalFile] = []
    for entry in as_list(data.get("files")):
        file_data = as_dict(entry)
        files.append(
            ProposalFile(
                target_path=as_str(file_data, "target_path"),
                content=as_str(file_data, "content"),
            )
        )
    return Proposal(
        id=as_str(data, "id"),
        source_flow=as_str(data, "source_flow"),
        source_run=as_str(data, "source_run"),
        kind=as_str(data, "kind"),
        name=as_str(data, "name"),
        status=as_str(data, "status"),
        created_at=as_str(data, "created_at"),
        updated_at=as_str(data, "updated_at"),
        decided_at=opt_str(data, "decided_at"),
        decided_by=opt_str(data, "decided_by"),
        claimed_by=opt_str(data, "claimed_by"),
        claimed_until=opt_str(data, "claimed_until"),
        applied_sha=opt_str(data, "applied_sha"),
        error=opt_str(data, "error"),
        files=files,
    )


def group_of(target: str) -> str:
    parts = target.split("/")
    if target.startswith("global/skills/") and len(parts) > 2:
        return parts[2]
    return parts[-1]


def main(proposal_id: str, reply: str) -> RevisionResult:
    if not UUID.fullmatch(proposal_id):
        raise ValueError("invalid proposal id")

    detail = as_dict(run_memoryctl(["proposal", "get", "--id", proposal_id]))
    current = to_proposal(detail.get("proposal"))
    files = {file["target_path"]: file["content"] for file in current["files"]}
    groups = {group_of(target) for target in files}

    applied: list[str] = []
    for match in FILE_BLOCK.finditer(reply):
        target = match.group("target").strip()
        known = target in files
        if not known and (group_of(target) not in groups or not target.startswith("global/skills/")):
            continue
        content = match.group("content")
        if not content.endswith("\n"):
            content += "\n"
        files[target] = content
        applied.append(target)

    if not applied:
        raise RuntimeError("no applicable ===FILE:=== blocks found in the reply")

    payload = json.dumps(
        {"files": [{"target_path": target, "content": content} for target, content in files.items()]}
    )
    revised = run_memoryctl(
        ["proposal", "revise", "--id", proposal_id, "--actor", os.environ["WM_USERNAME"]],
        stdin=payload,
    )
    return RevisionResult(applied=applied, proposal=to_proposal(revised))
