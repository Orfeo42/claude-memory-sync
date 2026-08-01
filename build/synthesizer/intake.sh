#!/bin/sh
data_dir="${SYNTH_DATA_DIR:-/data}"
claude_bin="${SYNTH_CLAUDE_BIN:-claude}"
model="${SYNTH_MODEL:-sonnet}"

. "$(dirname "$0")/lib.sh"

ensure_canonical_work

tab=$(printf '\t')

scratch_dir=$(mktemp -d)
trap 'rm -rf "$scratch_dir"' EXIT INT TERM

config_candidates="$scratch_dir/config_candidates"
memory_candidates="$scratch_dir/memory_candidates"
clients_meta="$scratch_dir/clients_meta"
failed_clients="$scratch_dir/failed_clients"
changed_projects="$scratch_dir/changed_projects"
commit_flag="$scratch_dir/commit_flag"
: > "$config_candidates"
: > "$memory_candidates"
: > "$clients_meta"
: > "$failed_clients"
: > "$changed_projects"
: > "$commit_flag"

clients_processed=0
projects_changed_total=0
config_changed=0

process_memory_project() {
  key="$1"
  proj_scratch="$scratch_dir/proj-$key"
  rm -rf "$proj_scratch"
  mkdir -p "$proj_scratch"
  prompt_file="$proj_scratch/prompt.txt"
  out_file="$proj_scratch/out.txt"
  memory_dir="$work_dir/projects/$key/memory"
  memory_index="$memory_dir/MEMORY.md"

  contributing_clients=$(awk -F"$tab" -v k="$key" '$1==k{print $2}' "$memory_candidates" | sort -u)

  {
    printf '%s\n' "You are a knowledge-intake judge for ONE project's memory corpus."
    printf '%s\n' "Candidates below come from one or more machines and may overlap with existing canonical entries."
    printf '%s\n' "Output ONLY genuinely NEW knowledge or entries that materially UPDATE an existing entry, as '===FILE: <name>.md===' blocks (full file content, frontmatter name/description/metadata.type preserved), terminated by '===END==='."
    printf '%s\n' "Reuse an existing filename when updating that entry; use a new kebab-case filename when adding new knowledge."
    printf '%s\n' "Never output '===DELETE:' ops: deletion authority belongs to the daily condense pass only."
    printf '%s\n' "If nothing qualifies as new or updated, output exactly 'NOTHING' and nothing else."
    printf '%s\n' "Never invent knowledge not present in the candidates. Never include secrets."
    printf '\n'
    if [ -f "$memory_index" ]; then
      printf 'INDEX:\n'
      cat "$memory_index"
      printf '\n'
    fi
    printf 'ENTRIES:\n'
    if [ -d "$memory_dir" ]; then
      for entry in "$memory_dir"/*.md; do
        [ -f "$entry" ] || continue
        entry_name=$(basename "$entry")
        [ "$entry_name" = "MEMORY.md" ] && continue
        printf -- '--- %s ---\n' "$entry_name"
        cat "$entry"
        printf '\n'
      done
    fi
    printf '\n'
    printf '%s\n' "$contributing_clients" | while IFS= read -r cid; do
      [ -z "$cid" ] && continue
      client_repo="$repos_dir/clients/$cid.git"
      client_head=$(awk -F"$tab" -v c="$cid" '$1==c{print $2}' "$clients_meta")
      printf 'CANDIDATES (from machine %s):\n' "$cid"
      awk -F"$tab" -v k="$key" -v c="$cid" '$1==k && $2==c{print $3}' "$memory_candidates" | while IFS= read -r cpath; do
        [ -z "$cpath" ] && continue
        cname=$(basename "$cpath")
        printf -- '--- %s ---\n' "$cname"
        git --git-dir="$client_repo" show "$client_head:$cpath"
        printf '\n'
      done
    done
  } > "$prompt_file"

  if ! "$claude_bin" -p --model "$model" < "$prompt_file" > "$out_file" 2>"$proj_scratch/err.txt"; then
    echo "intake.sh: claude invocation failed for project $key" >&2
    printf '%s\n' "$contributing_clients" | while IFS= read -r cid; do
      [ -z "$cid" ] && continue
      echo "$cid" >> "$failed_clients"
    done
    return 0
  fi

  ops_dir="$proj_scratch/ops"
  files_list="$proj_scratch/files.list"
  deletes_list="$proj_scratch/deletes.list"
  parse_llm_ops "$out_file" "$ops_dir" "$files_list" "$deletes_list"

  applied_any=0
  while IFS= read -r name; do
    [ -z "$name" ] && continue
    if ! validated=$(validate_op_filename "$name" "intake.sh: project $key" "$ops_dir"); then
      continue
    fi
    if scan_op_secrets "$ops_dir/$validated"; then
      echo "intake.sh: rejecting FILE op for '$validated' in project $key: possible secret detected" >&2
      continue
    fi
    mkdir -p "$memory_dir"
    cp "$ops_dir/$validated" "$memory_dir/$validated"
    applied_any=1
  done < "$files_list"

  while IFS= read -r name; do
    [ -z "$name" ] && continue
    echo "intake.sh: ignoring unexpected DELETE op for '$name' in project $key (intake never deletes)" >&2
  done < "$deletes_list"

  if [ "$applied_any" -eq 1 ]; then
    rebuild_memory_index "$memory_dir"
    projects_changed_total=$((projects_changed_total + 1))
    echo "$key" >> "$changed_projects"
  fi

  return 0
}

for client_repo in "$repos_dir"/clients/*.git; do
  [ -d "$client_repo" ] || continue
  client_id=$(basename "$client_repo")
  client_id=${client_id%.git}

  head=$(git --git-dir="$client_repo" rev-parse --verify --quiet refs/heads/main)
  [ -z "$head" ] && continue

  marker=$(git --git-dir="$client_repo" rev-parse --verify --quiet refs/intake/last-processed)
  base="$marker"
  [ -z "$base" ] && base=4b825dc642cb6eb9a060e54bf8d69288fbee4904

  if [ -n "$marker" ] && [ "$marker" = "$head" ]; then
    continue
  fi

  clients_processed=$((clients_processed + 1))
  printf '%s\t%s\t%s\n' "$client_id" "$head" "$base" >> "$clients_meta"

  git --git-dir="$client_repo" diff --name-status "$base" "$head" | while IFS= read -r line; do
    [ -z "$line" ] && continue
    path=$(printf '%s' "$line" | awk -F'\t' '{print $2}')
    case "$path" in
      global/*)
        ts=$(git --git-dir="$client_repo" log -1 --format=%aI "$base".."$head" -- "$path")
        printf '%s\t%s\t%s\n' "$path" "$client_id" "$ts" >> "$config_candidates"
        ;;
      projects/*/memory/*.md)
        base_name=$(basename "$path")
        if [ "$base_name" = "MEMORY.md" ]; then
          continue
        fi
        if git --git-dir="$client_repo" cat-file -e "$head:$path" 2>/dev/null; then
          project_key=$(printf '%s' "$path" | awk -F'/' '{print $2}')
          printf '%s\t%s\t%s\n' "$project_key" "$client_id" "$path" >> "$memory_candidates"
        fi
        ;;
      *)
        ;;
    esac
  done
done

if [ -s "$config_candidates" ]; then
  sort -t "$tab" -k1,1 -k3,3r "$config_candidates" | awk -F"$tab" '!seen[$1]++' > "$scratch_dir/config_winners"
else
  : > "$scratch_dir/config_winners"
fi

while IFS="$tab" read -r path winner_client ts; do
  [ -z "$path" ] && continue
  client_repo="$repos_dir/clients/$winner_client.git"
  client_head=$(awk -F"$tab" -v c="$winner_client" '$1==c{print $2}' "$clients_meta")
  dest="$work_dir/$path"
  if git --git-dir="$client_repo" cat-file -e "$client_head:$path" 2>/dev/null; then
    tmp_file="$scratch_dir/config_apply_tmp"
    git --git-dir="$client_repo" show "$client_head:$path" > "$tmp_file"
    if scan_op_secrets "$tmp_file"; then
      echo "intake.sh: rejecting config path '$path' from client '$winner_client': possible secret detected" >&2
      continue
    fi
    mkdir -p "$(dirname "$dest")"
    cp "$tmp_file" "$dest"
    config_changed=1
  else
    if [ -f "$dest" ]; then
      rm -f "$dest"
      config_changed=1
    fi
  fi
done < "$scratch_dir/config_winners"

if [ -s "$memory_candidates" ]; then
  cut -f1 "$memory_candidates" | sort -u > "$scratch_dir/project_keys"
else
  : > "$scratch_dir/project_keys"
fi

while IFS= read -r key; do
  [ -z "$key" ] && continue
  process_memory_project "$key"
done < "$scratch_dir/project_keys"

while IFS="$tab" read -r client_id chead cbase; do
  [ -z "$client_id" ] && continue
  client_repo="$repos_dir/clients/$client_id.git"
  if grep -Fqx "$client_id" "$failed_clients"; then
    continue
  fi
  git --git-dir="$client_repo" update-ref refs/intake/last-processed "$chead"
done < "$clients_meta"

sort -u "$changed_projects" > "$scratch_dir/changed_projects_sorted"
while IFS= read -r key; do
  [ -z "$key" ] && continue
  if with_canonical_lock commit_with_retry "$work_dir" "intake: $key" "projects/$key/memory"; then
    echo x >> "$commit_flag"
  else
    echo "intake.sh: failed to commit changes for project $key after 3 attempts, changes left staged" >&2
  fi
done < "$scratch_dir/changed_projects_sorted"

if [ "$config_changed" -eq 1 ]; then
  if with_canonical_lock commit_with_retry "$work_dir" "intake: config" "global"; then
    echo x >> "$commit_flag"
  else
    echo "intake.sh: failed to commit config changes after 3 attempts, changes left staged" >&2
  fi
fi

if [ -s "$commit_flag" ]; then
  with_canonical_lock git -C "$work_dir" push origin main
fi

printf 'intake pass done, clients_processed=%s projects_changed=%s\n' "$clients_processed" "$projects_changed_total"
