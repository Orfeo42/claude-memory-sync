import pathlib
import re
from typing import TypedDict

FILE_BLOCK = re.compile(r"^===FILE: (?P<target>.+?)===\n(?P<content>.*?)\n===END===", re.DOTALL | re.MULTILINE)


class RevisionResult(TypedDict):
    applied: list[str]
    content: str


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


def group_files(staging: pathlib.Path, group: str) -> dict[str, pathlib.Path]:
    files: dict[str, pathlib.Path] = {}
    for sidecar in sorted(staging.glob("*.path")):
        target = sidecar.read_text().strip()
        if group_of(target) != group:
            continue
        content_file = staging / sidecar.name[: -len(".path")]
        if content_file.is_file():
            files[target] = content_file
    return files


def split_frontmatter(content: str) -> tuple[str, str]:
    if not content.startswith("---\n"):
        return "", content
    end = content.find("\n---\n", 4)
    if end == -1:
        return "", content
    return content[4:end], content[end + 5 :]


def fence_language(target: str) -> str:
    if target.endswith(".sh"):
        return "bash"
    if target.endswith(".py"):
        return "python"
    return ""


def format_file(target: str, content: str) -> str:
    parts: list[str] = [f"**`{target}`**\n"]
    if target.endswith(".md"):
        frontmatter, body = split_frontmatter(content)
        if frontmatter:
            parts.append("```yaml\n" + frontmatter + "\n```\n")
        parts.append(body)
    else:
        parts.append("```" + fence_language(target) + "\n" + content + "\n```\n")
    return "\n".join(parts)


def main(run_id: str, group: str, reply: str) -> RevisionResult:
    if not re.fullmatch(r"[0-9a-f-]{36}", run_id):
        raise ValueError("invalid run id")
    staging = staging_dir(run_id)
    if staging is None:
        raise RuntimeError("no staging directory found for this run")

    files = group_files(staging, group)
    applied: list[str] = []
    for match in FILE_BLOCK.finditer(reply):
        target = match.group("target").strip()
        if group_of(target) != group:
            continue
        content = match.group("content")
        if not content.endswith("\n"):
            content += "\n"
        existing = files.get(target)
        if existing is not None:
            existing.write_text(content)
        else:
            if not target.startswith("global/skills/"):
                continue
            flat = target.replace("/", "__")
            (staging / flat).write_text(content)
            (staging / (flat + ".path")).write_text(target + "\n")
        applied.append(target)

    if not applied:
        raise RuntimeError("no applicable ===FILE:=== blocks found in the reply")

    refreshed = group_files(staging, group)
    content_md = "\n".join(format_file(target, path.read_text()) for target, path in refreshed.items())
    return RevisionResult(applied=applied, content=content_md)
