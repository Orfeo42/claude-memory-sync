import json
import os
import re
import subprocess
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"

UUID = re.compile(r"[0-9a-f-]{36}")

ALLOWED_MODELS = ("sonnet", "opus", "haiku")

INSTRUCTIONS = """You are helping a human reviewer refine a staged proposal (a Claude Code \
skill) before it is approved into the shared knowledge base. Discuss, answer questions, and \
propose improvements. When you want to propose a concrete revision, output the COMPLETE new \
content of each changed file as:

===FILE: <target-path>===
<full new file content>
===END===

Rules: only revise files belonging to this proposal (targets listed below). Keep frontmatter \
valid (name, description). Keep any Evidence section accurate — never invent evidence. Never \
include secrets. Outside the blocks, keep commentary short."""


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


class ChatReply(TypedDict):
    reply: str


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


def parse_history(history_json: str) -> list[tuple[str, str]]:
    parsed = cast(Json, json.loads(history_json))
    turns: list[tuple[str, str]] = []
    if not isinstance(parsed, list):
        return turns
    for entry in parsed:
        if not isinstance(entry, dict):
            continue
        role = entry.get("role")
        content = entry.get("content")
        if isinstance(role, str) and isinstance(content, str):
            turns.append((role, content))
    return turns


def main(
    proposal_id: str,
    message: str,
    history_json: str = "[]",
    model: str = "sonnet",
) -> ChatReply:
    if os.environ.get("ANTHROPIC_API_KEY"):
        raise RuntimeError("ANTHROPIC_API_KEY set — refusing (API credits)")
    if model not in ALLOWED_MODELS:
        raise ValueError(f"model must be one of {ALLOWED_MODELS}")
    if not UUID.fullmatch(proposal_id):
        raise ValueError("invalid proposal id")

    detail = as_dict(run_memoryctl(["proposal", "get", "--id", proposal_id]))
    proposal = to_proposal(detail.get("proposal"))
    if not proposal["files"]:
        raise RuntimeError("proposal has no files")

    parts: list[str] = [INSTRUCTIONS, "\n=== staged files ==="]
    for file in proposal["files"]:
        parts.append(f"--- {file['target_path']} ---")
        parts.append(file["content"])
    parts.append("\n=== conversation ===")
    for role, content in parse_history(history_json):
        parts.append(f"[{role}]\n{content}")
    parts.append(f"[user]\n{message}")
    parts.append("\n[assistant]")

    result = subprocess.run(
        ["claude", "-p", "--model", model],
        input="\n".join(parts),
        capture_output=True,
        text=True,
        timeout=600,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(f"claude failed: {result.stderr[-400:]}")
    return ChatReply(reply=result.stdout.strip())
