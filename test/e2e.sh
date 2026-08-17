#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_ID="$$"
NET="cms-e2e-net-${RUN_ID}"
SERVER_NAME="cms-e2e-server-${RUN_ID}"
SERVER_DATA_VOL="cms-e2e-server-data-${RUN_ID}"
STATE_VOL_A="cms-e2e-state-a-${RUN_ID}"
STATE_VOL_B="cms-e2e-state-b-${RUN_ID}"
WORKDIR="$(mktemp -d)"
TOKEN="e2e-token"

SLUG_A="-home-e2e-a"
SLUG_B="-home-e2e-b"

PASS=0
FAIL=0

CLAUDE_A="local claude content for machine-a"
CLAUDE_B="local claude content for machine-b"
RULE_A="rule content from machine-a"
SKILL_A="Demo skill body from machine-a."
ENTRY_A="Entry A content from machine-a."
ENTRY_B="Entry B content from machine-b."

cleanup() {
  docker rm -f "$SERVER_NAME" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  docker volume rm "$SERVER_DATA_VOL" >/dev/null 2>&1 || true
  docker volume rm "$STATE_VOL_A" >/dev/null 2>&1 || true
  docker volume rm "$STATE_VOL_B" >/dev/null 2>&1 || true
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

assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if printf '%s' "$haystack" | grep -qF "$needle"; then
    record 0 "$desc"
  else
    echo "  needle:   [$needle]"
    echo "  haystack: [$haystack]"
    record 1 "$desc"
  fi
}

build_images() {
  docker build -f "$ROOT/build/server.Dockerfile" -t memory-server:e2e "$ROOT"
  docker build -f "$ROOT/build/agent.Dockerfile" -t memory-agent:e2e "$ROOT"
}

build_memoryctl() {
  CGO_ENABLED=0 go build -o "$WORKDIR/memoryctl" "$ROOT/cmd/memoryctl"
}

start_server() {
  docker volume create "$SERVER_DATA_VOL" >/dev/null
  docker network create "$NET" >/dev/null
  docker run -d --name "$SERVER_NAME" --network "$NET" \
    -p 127.0.0.1:0:8080 \
    -e MEMORY_TOKEN="$TOKEN" \
    -v "$SERVER_DATA_VOL:/data" \
    memory-server:e2e >/dev/null
}

wait_for_server() {
  for _ in $(seq 1 30); do
    if docker exec "$SERVER_NAME" wget -qO- http://localhost:8080/v1/healthz >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "FAIL: server did not become healthy"
  exit 1
}

server_host_url() {
  local addr
  addr="$(docker port "$SERVER_NAME" 8080/tcp | head -n1)"
  printf 'http://%s' "$addr"
}

git_ls_tree() {
  local repo="$1"
  docker exec "$SERVER_NAME" git --git-dir="/data/repos/$repo" ls-tree -r main --name-only 2>/dev/null || true
}

git_show() {
  local repo="$1" rev="$2" path="$3"
  docker exec "$SERVER_NAME" git --git-dir="/data/repos/$repo" show "$rev:$path" 2>/dev/null || true
}

git_ref() {
  local repo="$1" ref="$2"
  docker exec "$SERVER_NAME" git --git-dir="/data/repos/$repo" rev-parse --verify --quiet "$ref" 2>/dev/null || true
}

make_fixture_a() {
  mkdir -p "$FIXTURE_A/rules" "$FIXTURE_A/skills/demo" "$FIXTURE_A/projects/$SLUG_A/memory"
  printf '%s\n' "$CLAUDE_A" > "$FIXTURE_A/CLAUDE.md"
  printf '%s\n' "$RULE_A" > "$FIXTURE_A/rules/test-rule.md"
  printf -- '---\nname: demo\ndescription: demo skill\n---\n%s\n' "$SKILL_A" > "$FIXTURE_A/skills/demo/SKILL.md"
  printf -- '---\nname: entry-a\ndescription: entry from machine-a\n---\n%s\n' "$ENTRY_A" > "$FIXTURE_A/projects/$SLUG_A/memory/entry-a.md"
  printf '# Memory index\n\n- [entry-a](entry-a.md) — entry from machine-a\n' > "$FIXTURE_A/projects/$SLUG_A/memory/MEMORY.md"
  chmod -R 777 "$FIXTURE_A"
}

make_fixture_b() {
  mkdir -p "$FIXTURE_B/projects/$SLUG_B/memory"
  printf '%s\n' "$CLAUDE_B" > "$FIXTURE_B/CLAUDE.md"
  printf -- '---\nname: entry-b\ndescription: entry from machine-b\n---\n%s\n' "$ENTRY_B" > "$FIXTURE_B/projects/$SLUG_B/memory/entry-b.md"
  printf '# Memory index\n\n- [entry-b](entry-b.md) — entry from machine-b\n' > "$FIXTURE_B/projects/$SLUG_B/memory/MEMORY.md"
  chmod -R 777 "$FIXTURE_B"
}

make_stub_ops() {
  cat > "$WORKDIR/stub-ops.txt" <<'EOF'
===FILE: merged-entry.md===
---
name: merged-entry
description: stub merged entry from the synthesis pipeline
metadata:
  type: reference
---
Stub merged content produced by the e2e test's ops stub.
===END===
EOF
}

run_agent() {
  local client_id="$1" fixture="$2" slug_prefix="$3" state_vol="$4"
  docker run --rm --network "$NET" \
    -e MEMORY_SERVER_URL="http://${SERVER_NAME}:8080" \
    -e MEMORY_TOKEN="$TOKEN" \
    -e MEMORY_CLIENT_ID="$client_id" \
    -e MEMORY_SLUG_PREFIX="$slug_prefix" \
    -e MEMORY_RUN_ONCE=true \
    -v "$fixture:/claude" \
    -v "$state_vol:/state" \
    memory-agent:e2e
}

run_canonical_update() {
  local logfile="$1"
  docker run --rm \
    -v "$SERVER_DATA_VOL:/data" \
    -v "$WORKDIR/memoryctl:/usr/local/bin/memoryctl:ro" \
    -v "$WORKDIR/stub-ops.txt:/stub-ops.txt:ro" \
    --entrypoint sh \
    alpine/git -c '
      set -e
      git config --global safe.directory "*"
      git config --global user.email e2e@example.com
      git config --global user.name e2e
      memoryctl ensure-work --repos-dir /data/repos --work-dir /data/work/canonical
      memoryctl ops-apply --memory-dir /data/work/canonical/projects/HOME/memory < /stub-ops.txt
      memoryctl commit --work-dir /data/work/canonical \
        --lock /data/state/canonical.lock \
        --message "synthesize: merged entry (e2e)" \
        --pathspec projects/HOME
      memoryctl push --work-dir /data/work/canonical --lock /data/state/canonical.lock
      echo "canonical update completed"
    ' > "$logfile" 2>&1
}

push_disallowed_path() {
  local repo_dir="$WORKDIR/host-repo-disallowed"
  rm -rf "$repo_dir"
  mkdir -p "$repo_dir/sessions"
  printf 'decoy session data\n' > "$repo_dir/sessions/evil.jsonl"
  git -C "$repo_dir" init -q -b main
  git -C "$repo_dir" config user.email "e2e@example.com"
  git -C "$repo_dir" config user.name "e2e"
  git -C "$repo_dir" add -A
  git -C "$repo_dir" commit -q -m "evil"
  git -C "$repo_dir" -c http.extraHeader="Authorization: Bearer $TOKEN" \
    push "$(server_host_url)/git/clients/machine-c.git" main
}

push_to_canonical() {
  local repo_dir="$WORKDIR/host-repo-canonical"
  rm -rf "$repo_dir"
  mkdir -p "$repo_dir"
  printf 'attempted direct canonical write\n' > "$repo_dir/CLAUDE.md"
  git -C "$repo_dir" init -q -b main
  git -C "$repo_dir" config user.email "e2e@example.com"
  git -C "$repo_dir" config user.name "e2e"
  git -C "$repo_dir" add -A
  git -C "$repo_dir" commit -q -m "attempt"
  git -C "$repo_dir" -c http.extraHeader="Authorization: Bearer $TOKEN" \
    push "$(server_host_url)/git/canonical.git" main
}

assertions_after_agent_a() {
  local tree
  tree="$(git_ls_tree clients/machine-a.git)"
  assert_contains "3: machine-a repo commit contains global/CLAUDE.md" "global/CLAUDE.md" "$tree"
  assert_contains "3: machine-a repo commit contains projects/HOME/memory/entry-a.md" "projects/HOME/memory/entry-a.md" "$tree"
  assert_contains "3: machine-a repo commit contains global/rules/test-rule.md" "global/rules/test-rule.md" "$tree"
  assert_contains "3: machine-a repo commit contains global/skills/demo/SKILL.md" "global/skills/demo/SKILL.md" "$tree"
}

assertions_after_agent_b() {
  local tree
  tree="$(git_ls_tree clients/machine-b.git)"
  assert_contains "4: machine-b repo commit contains global/CLAUDE.md" "global/CLAUDE.md" "$tree"
  assert_contains "4: machine-b repo commit contains projects/HOME/memory/entry-b.md" "projects/HOME/memory/entry-b.md" "$tree"
}

assertions_whitelist_rejection() {
  assert_false "5: push with disallowed path (sessions/evil.jsonl) is rejected" push_disallowed_path
  local main_ref
  main_ref="$(git_ref clients/machine-c.git refs/heads/main)"
  assert_eq "5: machine-c repo has no main ref after rejected push" "" "$main_ref"
}

assertions_canonical_push_blocked() {
  assert_false "6: push to canonical.git over http is rejected" push_to_canonical
}

assertions_after_canonical_update() {
  local log memory_tree
  log="$(cat "$WORKDIR/canonical-update.log")"
  echo "$log"
  assert_contains "7: canonical update completed" "canonical update completed" "$log"

  memory_tree="$(git_ls_tree canonical.git)"
  assert_contains "7: canonical has pipeline-produced projects/HOME/memory/merged-entry.md" "projects/HOME/memory/merged-entry.md" "$memory_tree"

  local memory_index
  memory_index="$(git_show canonical.git main projects/HOME/memory/MEMORY.md)"
  assert_contains "7: canonical MEMORY.md rebuilt with merged-entry reference" "merged-entry.md" "$memory_index"
}

assertions_after_downsync_a() {
  assert_true "8: agent A down-synced pipeline merged-entry.md into local project memory" \
    test -f "$FIXTURE_A/projects/$SLUG_A/memory/merged-entry.md"
  assert_contains "8: down-synced merged-entry.md has stub content" \
    "Stub merged content produced by the e2e test's ops stub." \
    "$(cat "$FIXTURE_A/projects/$SLUG_A/memory/merged-entry.md" 2>/dev/null || true)"
  assert_eq "8: agent A's own CLAUDE.md is untouched (local wins)" \
    "$CLAUDE_A" "$(head -n1 "$FIXTURE_A/CLAUDE.md")"
}

FIXTURE_A="$WORKDIR/fixture-a"
FIXTURE_B="$WORKDIR/fixture-b"

echo "== 1: build images =="
build_images
build_memoryctl

echo "== 2: start server =="
start_server
wait_for_server

echo "== 3: agent A first run =="
make_fixture_a
run_agent machine-a "$FIXTURE_A" "$SLUG_A" "$STATE_VOL_A"
assertions_after_agent_a

echo "== 4: agent B first run (same canonical project key) =="
make_fixture_b
run_agent machine-b "$FIXTURE_B" "$SLUG_B" "$STATE_VOL_B"
assertions_after_agent_b

echo "== 5: whitelist rejection on client push =="
assertions_whitelist_rejection

echo "== 6: canonical push blocked over http =="
assertions_canonical_push_blocked

echo "== 7: canonical update via memoryctl (as the Windmill flows do) =="
make_stub_ops
run_canonical_update "$WORKDIR/canonical-update.log"
assertions_after_canonical_update

echo "== 8: agent A down-sync picks up pipeline output =="
run_agent machine-a "$FIXTURE_A" "$SLUG_A" "$STATE_VOL_A"
assertions_after_downsync_a

echo "----"
echo "PASS: $PASS FAIL: $FAIL"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
