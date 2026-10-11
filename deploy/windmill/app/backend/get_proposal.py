import json
import re
import subprocess
from typing import TypeAlias, TypedDict, cast

Json: TypeAlias = "dict[str, Json] | list[Json] | str | int | float | bool | None"

UUID = re.compile(r"[0-9a-f-]{36}")


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


class ProposalEvent(TypedDict):
    id: int
    proposal_id: str
    from_status: str | None
    to_status: str
    actor: str
    note: str | None
    at: str


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


class ProposalDetail(TypedDict):
    proposal: Proposal
    events: list[ProposalEvent]


def to_event(value: Json) -> ProposalEvent:
    data = as_dict(value)
    event_id = data.get("id")
    if not isinstance(event_id, int):
        raise TypeError("expected integer field id")
    return ProposalEvent(
        id=event_id,
        proposal_id=as_str(data, "proposal_id"),
        from_status=opt_str(data, "from_status"),
        to_status=as_str(data, "to_status"),
        actor=as_str(data, "actor"),
        note=opt_str(data, "note"),
        at=as_str(data, "at"),
    )


def main(proposal_id: str) -> ProposalDetail:
    if not UUID.fullmatch(proposal_id):
        raise ValueError("invalid proposal id")
    data = as_dict(run_memoryctl(["proposal", "get", "--id", proposal_id]))
    return ProposalDetail(
        proposal=to_proposal(data.get("proposal")),
        events=[to_event(event) for event in as_list(data.get("events"))],
    )
