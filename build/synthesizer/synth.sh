#!/bin/sh
data_dir="${SYNTH_DATA_DIR:-/data}"
claude_bin="${SYNTH_CLAUDE_BIN:-claude}"
model="${SYNTH_MODEL:-sonnet}"

if [ ! -d "$data_dir/.git" ]; then
  echo "synth.sh: $data_dir is not a git repository (missing .git)" >&2
  exit 1
fi

git config --global --add safe.directory "$data_dir"
git config --global user.name memory-synthesizer
git config --global user.email memory-synthesizer@local

secret_pattern='(api[_-]?key|secret|password|token)[[:space:]]*[:=][[:space:]]*[^[:space:]]{8,}|ghp_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----'

extract_frontmatter_field() {
  awk -v field="$1" '
    NR==1 && $0=="---" { infm=1; next }
    infm && $0=="---" { exit }
    infm && index($0, field ":")==1 {
      print substr($0, length(field)+3)
      exit
    }
  ' "$2"
}

strip_quotes() {
  value="$1"
  case "$value" in
    \"*\") value=${value#\"}; value=${value%\"} ;;
    \'*\') value=${value#\'}; value=${value%\'} ;;
  esac
  printf '%s' "$value"
}

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
  mkdir -p "$ops_dir"
  : > "$files_list"
  : > "$deletes_list"

  awk -v opsdir="$ops_dir" -v fileslist="$files_list" -v deleteslist="$deletes_list" '
    BEGIN { mode = "" }
    index($0, "===FILE: ")==1 && $0 ~ /===$/ {
      name = substr($0, 10)
      name = substr(name, 1, length(name) - 3)
      outfile = opsdir "/" name
      print name >> fileslist
      close(fileslist)
      mode = "file"
      next
    }
    $0 == "===END===" {
      mode = ""
      next
    }
    index($0, "===DELETE: ")==1 && $0 ~ /===$/ {
      name = substr($0, 12)
      name = substr(name, 1, length(name) - 3)
      print name >> deleteslist
      close(deleteslist)
      next
    }
    mode == "file" { print $0 >> outfile; next }
  ' "$out_file"

  accepted_files="$scratch_dir/accepted_files.list"
  accepted_deletes="$scratch_dir/accepted_deletes.list"
  : > "$accepted_files"
  : > "$accepted_deletes"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    normalized=$(printf '%s' "$name" | tr '_[:upper:]' '-[:lower:]')
    if [ "$normalized" != "$name" ] && [ -f "$ops_dir/$name" ]; then
      mv -- "$ops_dir/$name" "$ops_dir/$normalized"
    fi
    name=$normalized
    if [ "$name" = "memory.md" ]; then
      echo "synth.sh: rejecting FILE op targeting MEMORY.md for project $project_key" >&2
      continue
    fi
    if ! printf '%s\n' "$name" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*\.md$'; then
      echo "synth.sh: rejecting FILE op with invalid filename '$name' for project $project_key" >&2
      continue
    fi
    if grep -Eqi "$secret_pattern" "$ops_dir/$name"; then
      echo "synth.sh: rejecting FILE op for '$name' in project $project_key: possible secret detected" >&2
      continue
    fi
    echo "$name" >> "$accepted_files"
  done < "$files_list"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    name=$(printf '%s' "$name" | tr '_[:upper:]' '-[:lower:]')
    if [ "$name" = "memory.md" ]; then
      echo "synth.sh: rejecting DELETE op targeting MEMORY.md for project $project_key" >&2
      continue
    fi
    if ! printf '%s\n' "$name" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*\.md$'; then
      echo "synth.sh: rejecting DELETE op with invalid filename '$name' for project $project_key" >&2
      continue
    fi
    if [ ! -f "$memory_dir/$name" ]; then
      echo "synth.sh: rejecting DELETE op for '$name' in project $project_key: target does not exist" >&2
      continue
    fi
    echo "$name" >> "$accepted_deletes"
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

  header_file="$scratch_dir/header.txt"
  if [ -f "$memory_index" ]; then
    awk '/^- \[/{exit} {print}' "$memory_index" > "$header_file"
  else
    printf '# Memory index\n\n' > "$header_file"
  fi

  new_index="$scratch_dir/MEMORY.md.new"
  cp "$header_file" "$new_index"

  for entry in "$memory_dir"/*.md; do
    [ -f "$entry" ] || continue
    entry_name=$(basename "$entry")
    [ "$entry_name" = "MEMORY.md" ] && continue
    fm_name=$(extract_frontmatter_field name "$entry")
    fm_description=$(extract_frontmatter_field description "$entry")
    fm_description=$(strip_quotes "$fm_description")
    printf -- '- [%s](%s) — %s\n' "$fm_name" "$entry_name" "$fm_description" >> "$new_index"
  done

  mv "$new_index" "$memory_index"

  attempt=1
  committed=0
  while [ "$attempt" -le 3 ]; do
    git -C "$data_dir" add -A -- "$memory_dir"
    if git -C "$data_dir" commit -m "synthesize: $project_key" -- "$memory_dir" >/dev/null 2>&1; then
      committed=1
      break
    fi
    attempt=$((attempt + 1))
    sleep 2
  done

  if [ "$committed" -eq 1 ]; then
    changed=1
  else
    echo "synth.sh: failed to commit changes for project $project_key after 3 attempts, changes left staged" >&2
  fi

  rm -rf "$scratch_dir"
  trap - EXIT INT TERM
  return 0
}

projects_changed=0

for memory_dir in "$data_dir"/canonical/projects/*/memory; do
  [ -d "$memory_dir" ] || continue
  project_key=$(basename "$(dirname "$memory_dir")")
  process_project "$memory_dir" "$project_key"
  if [ "$changed" -eq 1 ]; then
    projects_changed=$((projects_changed + 1))
  fi
done

printf 'synthesis pass done, projects_changed=%s\n' "$projects_changed"
