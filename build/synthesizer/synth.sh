#!/bin/sh
data_dir="${SYNTH_DATA_DIR:-/data}"
claude_bin="${SYNTH_CLAUDE_BIN:-claude}"
model="${SYNTH_MODEL:-sonnet}"

. "$(dirname "$0")/lib.sh"

ensure_canonical_work

process_project() {
  memory_dir="$1"
  project_key="$2"
  changed=0

  entry_count=$(find "$memory_dir" -maxdepth 1 -type f -name '*.md' ! -name 'MEMORY.md' | wc -l | tr -d ' ')
  if [ "$entry_count" -lt 2 ]; then
    return 0
  fi

  scratch_dir=$(mktemp -d)
  trap 'rm -rf "$scratch_dir"' EXIT INT TERM

  memory_index="$memory_dir/MEMORY.md"
  prompt_file="$scratch_dir/prompt.txt"
  out_file="$scratch_dir/out.txt"

  {
    printf '%s\n' "You are a memory-corpus condenser."
    printf '%s\n' "Input is the full set of memory entries for ONE project (each with a filename header) plus its MEMORY.md index."
    printf '%s\n' "Merge semantic duplicates into one entry. Rewrite contradictory or vague entries into one clear form."
    printf '%s\n' "Preserve the frontmatter format (name/description/metadata.type) and [[wikilinks]]."
    printf '%s\n' "Prefer keeping existing filenames; when merging file A and file B, keep the better filename and delete the other."
    printf '%s\n' "Only output what CHANGES, using exactly this contract:"
    printf '%s\n' "For a created or rewritten file, output a line '===FILE: <name>.md===', then the full new file content, then a line '===END==='."
    printf '%s\n' "For a deleted file, output a line '===DELETE: <name>.md===' on its own line."
    printf '%s\n' "If nothing should change, output exactly 'NOTHING' and nothing else."
    printf '%s\n' "Never invent knowledge not present in the entries. Never include secrets."
    printf '\n'
    if [ -f "$memory_index" ]; then
      printf 'INDEX:\n'
      cat "$memory_index"
      printf '\n'
    fi
    printf 'ENTRIES:\n'
    for entry in "$memory_dir"/*.md; do
      [ -f "$entry" ] || continue
      entry_name=$(basename "$entry")
      [ "$entry_name" = "MEMORY.md" ] && continue
      printf -- '--- %s ---\n' "$entry_name"
      cat "$entry"
      printf '\n'
    done
  } > "$prompt_file"

  if ! "$claude_bin" -p --model "$model" < "$prompt_file" > "$out_file" 2>"$scratch_dir/err.txt"; then
    echo "synth.sh: claude invocation failed for project $project_key" >&2
    rm -rf "$scratch_dir"
    trap - EXIT INT TERM
    return 0
  fi

  ops_dir="$scratch_dir/ops"
  files_list="$scratch_dir/files.list"
  deletes_list="$scratch_dir/deletes.list"
  parse_llm_ops "$out_file" "$ops_dir" "$files_list" "$deletes_list"

  accepted_files="$scratch_dir/accepted_files.list"
  accepted_deletes="$scratch_dir/accepted_deletes.list"
  : > "$accepted_files"
  : > "$accepted_deletes"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    if ! validated=$(validate_op_filename "$name" "synth.sh: project $project_key" "$ops_dir"); then
      continue
    fi
    if scan_op_secrets "$ops_dir/$validated"; then
      echo "synth.sh: rejecting FILE op for '$validated' in project $project_key: possible secret detected" >&2
      continue
    fi
    echo "$validated" >> "$accepted_files"
  done < "$files_list"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    if ! validated=$(validate_op_filename "$name" "synth.sh: project $project_key"); then
      continue
    fi
    if [ ! -f "$memory_dir/$validated" ]; then
      echo "synth.sh: rejecting DELETE op for '$validated' in project $project_key: target does not exist" >&2
      continue
    fi
    echo "$validated" >> "$accepted_deletes"
  done < "$deletes_list"

  accepted_files_count=$(wc -l < "$accepted_files" | tr -d ' ')
  accepted_deletes_count=$(wc -l < "$accepted_deletes" | tr -d ' ')
  accepted_total=$((accepted_files_count + accepted_deletes_count))

  if [ "$accepted_total" -eq 0 ]; then
    rm -rf "$scratch_dir"
    trap - EXIT INT TERM
    return 0
  fi

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    cp "$ops_dir/$name" "$memory_dir/$name"
  done < "$accepted_files"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    rm -f "$memory_dir/$name"
  done < "$accepted_deletes"

  rebuild_memory_index "$memory_dir"

  if with_canonical_lock commit_with_retry "$work_dir" "synthesize: $project_key" "$memory_dir"; then
    changed=1
  else
    echo "synth.sh: failed to commit changes for project $project_key after 3 attempts, changes left staged" >&2
  fi

  rm -rf "$scratch_dir"
  trap - EXIT INT TERM
  return 0
}

projects_changed=0

for memory_dir in "$work_dir"/projects/*/memory; do
  [ -d "$memory_dir" ] || continue
  project_key=$(basename "$(dirname "$memory_dir")")
  process_project "$memory_dir" "$project_key"
  if [ "$changed" -eq 1 ]; then
    projects_changed=$((projects_changed + 1))
  fi
done

if [ "$projects_changed" -gt 0 ]; then
  with_canonical_lock git -C "$work_dir" push origin main
fi

printf 'synthesis pass done, projects_changed=%s\n' "$projects_changed"
