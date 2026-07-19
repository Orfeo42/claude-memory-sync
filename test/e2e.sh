#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$$"
NET="cms-e2e-net-${RUN_ID}"
SERVER_NAME="cms-e2e-server-${RUN_ID}"
WORKDIR="$(mktemp -d)"

PASS=0
FAIL=0

LOCAL_CLAUDE_A="local claude content for machine-a"
CANONICAL_CLAUDE="canonical claude content seeded on hub"
LOCAL_RULE_X="rule x content"
LOCAL_MEMORY="proj1 memory content v1"
CANONICAL_EXTRA="canonical extra rule content"
LOCAL_CLAUDE_B="local claude content for machine-b"
LOCAL_RULE_Y="rule y content"
LOCAL_NOTES_B="proj2 notes content"

cleanup() {
  docker rm -f "$SERVER_NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

record() {
  local status="$1" desc="$2"
  if [ "$status" -eq 0 ]; then
    echo "PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc"
    FAIL=$((FAIL + 1))
  fi
}

assert_true() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    record 0 "$desc"
  else
    record 1 "$desc"
  fi
}

assert_false() {
  local desc="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    record 1 "$desc"
  else
    record 0 "$desc"
  fi
}

assert_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    record 0 "$desc"
  else
    echo "  expected: [$expected]"
    echo "  actual:   [$actual]"
    record 1 "$desc"
  fi
}

server_cat() {
  docker exec "$SERVER_NAME" cat "$1" 2>/dev/null || true
}

build_images() {
  docker build -f "$ROOT/build/server.Dockerfile" -t memory-server:e2e "$ROOT"
  docker build -f "$ROOT/build/agent.Dockerfile" -t memory-agent:e2e "$ROOT"
}

start_server() {
  mkdir -p "$WORKDIR/server-data"
  chmod 777 "$WORKDIR/server-data"
  docker network create "$NET" >/dev/null
  docker run -d --name "$SERVER_NAME" --network "$NET" \
    -e MEMORY_TOKEN=e2etoken \
    -v "$WORKDIR/server-data:/data" \
    memory-server:e2e >/dev/null
}

wait_for_server() {
  local i
  for i in $(seq 1 30); do
    if docker exec "$SERVER_NAME" wget -qO- http://localhost:8080/v1/healthz >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "FAIL: server did not become healthy"
  exit 1
}

seed_canonical() {
  docker exec "$SERVER_NAME" mkdir -p /data/canonical/global/rules
  printf '%s' "$CANONICAL_CLAUDE" | docker exec -i "$SERVER_NAME" sh -c 'cat > /data/canonical/global/CLAUDE.md'
  printf '%s' "$CANONICAL_EXTRA" | docker exec -i "$SERVER_NAME" sh -c 'cat > /data/canonical/global/rules/extra.md'
  docker exec "$SERVER_NAME" git -C /data add -A
  docker exec "$SERVER_NAME" git -C /data commit -m seed >/dev/null
}

make_fixture_a() {
  mkdir -p "$FIXTURE_A/rules" "$FIXTURE_A/projects/-home-e2e-proj1/memory"
  printf '%s' "$LOCAL_CLAUDE_A" > "$FIXTURE_A/CLAUDE.md"
  printf '%s' "$LOCAL_RULE_X" > "$FIXTURE_A/rules/x.md"
  printf '%s' "$LOCAL_MEMORY" > "$FIXTURE_A/projects/-home-e2e-proj1/memory/MEMORY.md"
  printf '%s' "decoy session data" > "$FIXTURE_A/projects/-home-e2e-proj1/session.jsonl"
  chmod -R 777 "$FIXTURE_A"
}

make_fixture_b() {
  mkdir -p "$FIXTURE_B/rules" "$FIXTURE_B/projects/-home-e2e-proj2/memory"
  printf '%s' "$LOCAL_CLAUDE_B" > "$FIXTURE_B/CLAUDE.md"
  printf '%s' "$LOCAL_RULE_Y" > "$FIXTURE_B/rules/y.md"
  printf '%s' "$LOCAL_NOTES_B" > "$FIXTURE_B/projects/-home-e2e-proj2/memory/NOTES.md"
  chmod -R 777 "$FIXTURE_B"
}

run_agent() {
  local client_id="$1" fixture="$2" state="$3"
  mkdir -p "$state"
  chmod -R 777 "$state"
  docker run --rm --network "$NET" \
    -e MEMORY_SERVER_URL="http://${SERVER_NAME}:8080" \
    -e MEMORY_TOKEN=e2etoken \
    -e MEMORY_CLIENT_ID="$client_id" \
    -e MEMORY_SLUG_PREFIX=-home-e2e \
    -e MEMORY_RUN_ONCE=true \
    -v "$fixture:/claude" \
    -v "$state:/state" \
    memory-agent:e2e
}

assertions_after_agent_a() {
  assert_true "a: machine-a MEMORY.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-a/projects/HOME-proj1/memory/MEMORY.md
  assert_eq "a: machine-a MEMORY.md content matches" \
    "$LOCAL_MEMORY" "$(server_cat /data/clients/machine-a/projects/HOME-proj1/memory/MEMORY.md)"

  assert_true "b: machine-a global/CLAUDE.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-a/global/CLAUDE.md
  assert_true "b: machine-a global/rules/x.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-a/global/rules/x.md

  local found
  found="$(docker exec "$SERVER_NAME" sh -c 'find /data/clients -name session.jsonl')"
  assert_eq "c: no session.jsonl under /data/clients" "" "$found"

  assert_eq "d: fixture CLAUDE.md unchanged (local wins)" \
    "$LOCAL_CLAUDE_A" "$(cat "$FIXTURE_A/CLAUDE.md")"
  assert_eq "d: fixture received canonical rules/extra.md" \
    "$CANONICAL_EXTRA" "$(cat "$FIXTURE_A/rules/extra.md" 2>/dev/null || true)"
}

assertions_after_agent_b() {
  assert_true "e: machine-b CLAUDE.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-b/global/CLAUDE.md
  assert_eq "e: machine-b CLAUDE.md content matches" \
    "$LOCAL_CLAUDE_B" "$(server_cat /data/clients/machine-b/global/CLAUDE.md)"
  assert_true "e: machine-b rules/y.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-b/global/rules/y.md
  assert_true "e: machine-b NOTES.md exists on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-b/projects/HOME-proj2/memory/NOTES.md
  assert_eq "e: machine-b NOTES.md content matches" \
    "$LOCAL_NOTES_B" "$(server_cat /data/clients/machine-b/projects/HOME-proj2/memory/NOTES.md)"

  assert_eq "e: machine-a namespace untouched by machine-b run (content)" \
    "$LOCAL_CLAUDE_A" "$(server_cat /data/clients/machine-a/global/CLAUDE.md)"
  assert_eq "e: machine-a namespace untouched by machine-b run (mtime+size)" \
    "$MACHINE_A_STAT_BEFORE" "$(docker exec "$SERVER_NAME" stat -c '%Y %s' /data/clients/machine-a/global/CLAUDE.md)"

  assert_eq "e: machine-b fixture received canonical rules/extra.md" \
    "$CANONICAL_EXTRA" "$(cat "$FIXTURE_B/rules/extra.md" 2>/dev/null || true)"
}

assertions_after_delete() {
  assert_false "f: machine-a MEMORY.md deleted on server" \
    docker exec "$SERVER_NAME" test -f /data/clients/machine-a/projects/HOME-proj1/memory/MEMORY.md

  local deleted
  deleted="$(docker exec "$SERVER_NAME" git -C /data log --diff-filter=D --name-only --pretty=format: -- clients/machine-a/projects/HOME-proj1/memory/MEMORY.md)"
  assert_true "f: deletion recoverable in git log" test -n "$deleted"
}

assertions_git_log() {
  local log
  log="$(docker exec "$SERVER_NAME" git -C /data log --oneline --all)"
  if printf '%s' "$log" | grep -q "sync: machine-a "; then
    record 0 "g: commit messages present for machine-a"
  else
    record 1 "g: commit messages present for machine-a"
  fi
  if printf '%s' "$log" | grep -q "sync: machine-b "; then
    record 0 "g: commit messages present for machine-b"
  else
    record 1 "g: commit messages present for machine-b"
  fi
}

FIXTURE_A="$WORKDIR/fixture-a"
FIXTURE_B="$WORKDIR/fixture-b"
STATE_A="$WORKDIR/state-a"
STATE_B="$WORKDIR/state-b"

build_images
start_server
wait_for_server
seed_canonical

make_fixture_a
run_agent machine-a "$FIXTURE_A" "$STATE_A"
assertions_after_agent_a

MACHINE_A_STAT_BEFORE="$(docker exec "$SERVER_NAME" stat -c '%Y %s' /data/clients/machine-a/global/CLAUDE.md)"

make_fixture_b
run_agent machine-b "$FIXTURE_B" "$STATE_B"
assertions_after_agent_b

rm -f "$FIXTURE_A/projects/-home-e2e-proj1/memory/MEMORY.md"
run_agent machine-a "$FIXTURE_A" "$STATE_A"
assertions_after_delete

assertions_git_log

echo "----"
echo "PASS: $PASS FAIL: $FAIL"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
