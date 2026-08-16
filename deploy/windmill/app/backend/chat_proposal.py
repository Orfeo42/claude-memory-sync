import json
import os
import pathlib
import re
import subprocess
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"

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


class ChatReply(TypedDict):
    reply: str


def group_of(target: str) -> str:
    parts = target.split("/")
    if target.startswith("global/skills/") and len(parts) > 2:
        return parts[2]
    return parts[-1]


def staging_dir(run_id: str) -> pathlib.Path | None:
    for candidate in (pathlib.Path("/data/state/proposals") / run_id, pathlib.Path("/tmp/proposals")):
        if candidate.is_dir():
            return candidate
    return None


def group_files(staging: pathlib.Path, group: str) -> list[tuple[str, pathlib.Path]]:
    files: list[tuple[str, pathlib.Path]] = []
    for sidecar in sorted(staging.glob("*.path")):
        target = sidecar.read_text().strip()
        if group_of(target) != group:
            continue
        content_file = staging / sidecar.name[: -len(".path")]
        if content_file.is_file():
            files.append((target, content_file))
    return files


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
    run_id: str,
    group: str,
    message: str,
    history_json: str = "[]",
    model: str = "sonnet",
) -> ChatReply:
    if os.environ.get("ANTHROPIC_API_KEY"):
        raise RuntimeError("ANTHROPIC_API_KEY set — refusing (API credits)")
    if model not in ALLOWED_MODELS:
        raise ValueError(f"model must be one of {ALLOWED_MODELS}")
    if not re.fullmatch(r"[0-9a-f-]{36}", run_id):
        raise ValueError("invalid run id")

    staging = staging_dir(run_id)
    if staging is None:
        raise RuntimeError("no staging directory found for this run")
    files = group_files(staging, group)
    if not files:
        raise RuntimeError(f"no staged files found for proposal '{group}'")

    parts: list[str] = [INSTRUCTIONS, "\n=== staged files ==="]
    for target, content_file in files:
        parts.append(f"--- {target} ---")
        parts.append(content_file.read_text())
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
