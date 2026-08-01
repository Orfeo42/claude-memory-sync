#!/bin/sh
set -eu

overall_fail=0

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
intake_sh="$repo_root/build/synthesizer/intake.sh"

root_tmp=$(mktemp -d)
cleanup() {
  rm -rf "$root_tmp"
}
trap cleanup EXIT INT TERM

assert() {
  description="$1"
  condition="$2"
  if [ "$condition" = "0" ]; then
    printf 'PASS: %s\n' "$description"
  else
    printf 'FAIL: %s\n' "$description"
    overall_fail=1
  fi
}

stub_dir="$root_tmp/stub"
mkdir -p "$stub_dir"
stub_claude="$stub_dir/claude"

{
  printf '%s\n' "#!/bin/sh"
  printf '%s\n' 'cat >/dev/null'
  printf '%s\n' 'if [ -n "${STUB_CLAUDE_OUTPUT:-}" ] && [ -f "$STUB_CLAUDE_OUTPUT" ]; then'
  printf '%s\n' '  cat "$STUB_CLAUDE_OUTPUT"'
  printf '%s\n' 'fi'
  printf '%s\n' 'exit 0'
} > "$stub_claude"
chmod +x "$stub_claude"

fixtures_dir="$root_tmp/fixtures"
mkdir -p "$fixtures_dir"

accept_fixture="$fixtures_dir/accept.txt"
{
  printf '%s\n' "===FILE: stubbed-entry.md==="
  printf '%s\n' "---"
  printf '%s\n' "name: stubbed-entry"
  printf '%s\n' "description: a stubbed accepted entry"
  printf '%s\n' "metadata:"
  printf '%s\n' "  type: reference"
  printf '%s\n' "---"
  printf '%s\n' "stubbed body content"
  printf '%s\n' "===END==="
} > "$accept_fixture"

nothing_fixture="$fixtures_dir/nothing.txt"
printf '%s\n' "NOTHING" > "$nothing_fixture"

scn1_data="$root_tmp/scn1/data"
scn1_clone="$root_tmp/scn1/clone"
mkdir -p "$scn1_data" "$scn1_clone"

client_repo_1="$scn1_data/repos/clients/machine-a.git"
mkdir -p "$(dirname "$client_repo_1")"
git init --quiet --bare --initial-branch=main "$client_repo_1"

git init --quiet --initial-branch=main "$scn1_clone"
git -C "$scn1_clone" config user.name test-client
git -C "$scn1_clone" config user.email test-client@local
git -C "$scn1_clone" remote add origin "$client_repo_1"

mkdir -p "$scn1_clone/global/rules" "$scn1_clone/projects/testproj/memory"
{
  printf '%s\n' "# Test rule"
  printf '%s\n' "content"
} > "$scn1_clone/global/rules/test.md"

{
  printf '%s\n' "---"
  printf '%s\n' "name: some-entry"
  printf '%s\n' "description: a test entry"
  printf '%s\n' "metadata:"
  printf '%s\n' "  type: reference"
  printf '%s\n' "---"
  printf '%s\n' "body text here"
} > "$scn1_clone/projects/testproj/memory/some-entry.md"

git -C "$scn1_clone" add -A
git -C "$scn1_clone" commit --quiet -m "seed"
git -C "$scn1_clone" push --quiet origin main

SYNTH_DATA_DIR="$scn1_data" SYNTH_CLAUDE_BIN="$stub_claude" STUB_CLAUDE_OUTPUT="$accept_fixture" sh "$intake_sh" >"$root_tmp/scn1_run1.log" 2>&1 || true

canonical_bare_1="$scn1_data/repos/canonical.git"
work_1="$scn1_data/work/canonical"

if [ -f "$work_1/global/rules/test.md" ] && grep -q "content" "$work_1/global/rules/test.md"; then
  assert "scenario1: config path applied to canonical work clone" 0
else
  assert "scenario1: config path applied to canonical work clone" 1
fi

if [ -f "$work_1/projects/testproj/memory/stubbed-entry.md" ] && grep -q "stubbed body content" "$work_1/projects/testproj/memory/stubbed-entry.md"; then
  assert "scenario1: LLM-accepted memory entry applied" 0
else
  assert "scenario1: LLM-accepted memory entry applied" 1
fi

if [ -f "$work_1/projects/testproj/memory/MEMORY.md" ] && grep -q "stubbed-entry.md" "$work_1/projects/testproj/memory/MEMORY.md"; then
  assert "scenario1: MEMORY.md index references stubbed-entry.md" 0
else
  assert "scenario1: MEMORY.md index references stubbed-entry.md" 1
fi

if [ -n "$(git --git-dir="$canonical_bare_1" log --oneline 2>/dev/null)" ]; then
  assert "scenario1: canonical bare repo received a commit" 0
else
  assert "scenario1: canonical bare repo received a commit" 1
fi

marker_1=$(git --git-dir="$client_repo_1" rev-parse --verify --quiet refs/intake/last-processed || true)
head_1=$(git --git-dir="$client_repo_1" rev-parse --verify --quiet refs/heads/main || true)
if [ -n "$marker_1" ] && [ "$marker_1" = "$head_1" ]; then
  assert "scenario1: client marker advanced to head" 0
else
  assert "scenario1: client marker advanced to head" 1
fi

head_before_second_run=$(git --git-dir="$canonical_bare_1" rev-parse --verify --quiet HEAD || true)

SYNTH_DATA_DIR="$scn1_data" SYNTH_CLAUDE_BIN="$stub_claude" STUB_CLAUDE_OUTPUT="$accept_fixture" sh "$intake_sh" >"$root_tmp/scn1_run2.log" 2>&1 || true

head_after_second_run=$(git --git-dir="$canonical_bare_1" rev-parse --verify --quiet HEAD || true)

if [ "$head_before_second_run" = "$head_after_second_run" ]; then
  assert "scenario1: second run is a no-op (marker skip)" 0
else
  assert "scenario1: second run is a no-op (marker skip)" 1
fi

scn2_data="$root_tmp/scn2/data"
scn2_clone="$root_tmp/scn2/clone"
mkdir -p "$scn2_data" "$scn2_clone"

client_repo_2="$scn2_data/repos/clients/machine-a.git"
mkdir -p "$(dirname "$client_repo_2")"
git init --quiet --bare --initial-branch=main "$client_repo_2"

git init --quiet --initial-branch=main "$scn2_clone"
git -C "$scn2_clone" config user.name test-client
git -C "$scn2_clone" config user.email test-client@local
git -C "$scn2_clone" remote add origin "$client_repo_2"

mkdir -p "$scn2_clone/global/rules" "$scn2_clone/projects/testproj/memory"
{
  printf '%s\n' "# Test rule two"
  printf '%s\n' "content two"
} > "$scn2_clone/global/rules/test2.md"

{
  printf '%s\n' "---"
  printf '%s\n' "name: candidate2"
  printf '%s\n' "description: a candidate that will be rejected as NOTHING"
  printf '%s\n' "metadata:"
  printf '%s\n' "  type: reference"
  printf '%s\n' "---"
  printf '%s\n' "candidate body text"
} > "$scn2_clone/projects/testproj/memory/candidate2.md"

git -C "$scn2_clone" add -A
git -C "$scn2_clone" commit --quiet -m "seed"
git -C "$scn2_clone" push --quiet origin main

SYNTH_DATA_DIR="$scn2_data" SYNTH_CLAUDE_BIN="$stub_claude" STUB_CLAUDE_OUTPUT="$nothing_fixture" sh "$intake_sh" >"$root_tmp/scn2_run1.log" 2>&1 || true

work_2="$scn2_data/work/canonical"

if [ -f "$work_2/global/rules/test2.md" ] && grep -q "content two" "$work_2/global/rules/test2.md"; then
  assert "scenario2: config path applied even though LLM said NOTHING" 0
else
  assert "scenario2: config path applied even though LLM said NOTHING" 1
fi

memory_entry_count=$(find "$work_2/projects/testproj/memory" -maxdepth 1 -type f -name '*.md' 2>/dev/null | wc -l | tr -d ' ')
if [ "$memory_entry_count" = "0" ]; then
  assert "scenario2: no memory entry applied when LLM said NOTHING" 0
else
  assert "scenario2: no memory entry applied when LLM said NOTHING" 1
fi

marker_2=$(git --git-dir="$client_repo_2" rev-parse --verify --quiet refs/intake/last-processed || true)
head_2=$(git --git-dir="$client_repo_2" rev-parse --verify --quiet refs/heads/main || true)
if [ -n "$marker_2" ] && [ "$marker_2" = "$head_2" ]; then
  assert "scenario2: client marker advances on NOTHING (not a failure)" 0
else
  assert "scenario2: client marker advances on NOTHING (not a failure)" 1
fi

if [ "$overall_fail" = "0" ]; then
  printf 'SUMMARY: all assertions passed\n'
  exit 0
else
  printf 'SUMMARY: one or more assertions failed\n'
  exit 1
fi
