#!/bin/sh
work_dir="$data_dir/work/canonical"
repos_dir="$data_dir/repos"

# keep in sync with internal/githook
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

parse_llm_ops() {
  out_file="$1"
  ops_dir="$2"
  files_list="$3"
  deletes_list="$4"
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
}

validate_op_filename() {
  name="$1"
  context_label="$2"
  ops_dir_arg="$3"
  normalized=$(printf '%s' "$name" | tr '_[:upper:]' '-[:lower:]')
  if [ -n "$ops_dir_arg" ] && [ "$normalized" != "$name" ] && [ -f "$ops_dir_arg/$name" ]; then
    mv -- "$ops_dir_arg/$name" "$ops_dir_arg/$normalized"
  fi
  name=$normalized
  if [ "$name" = "memory.md" ]; then
    echo "$context_label: rejecting op targeting MEMORY.md" >&2
    return 1
  fi
  if ! printf '%s\n' "$name" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*\.md$'; then
    echo "$context_label: rejecting op with invalid filename '$name'" >&2
    return 1
  fi
  printf '%s' "$name"
  return 0
}

scan_op_secrets() {
  grep -Eqi "$secret_pattern" "$1"
}

rebuild_memory_index() {
  memory_dir="$1"
  memory_index="$memory_dir/MEMORY.md"
  rmi_scratch_dir=$(mktemp -d)

  header_file="$rmi_scratch_dir/header.txt"
  if [ -f "$memory_index" ]; then
    awk '/^- \[/{exit} {print}' "$memory_index" > "$header_file"
  else
    printf '# Memory index\n\n' > "$header_file"
  fi

  new_index="$rmi_scratch_dir/MEMORY.md.new"
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
  rm -rf "$rmi_scratch_dir"
}

ensure_canonical_work() {
  mkdir -p "$repos_dir/clients" "$data_dir/state"

  if [ ! -d "$repos_dir/canonical.git" ]; then
    git init --bare --initial-branch=main "$repos_dir/canonical.git"
  fi

  if [ ! -d "$work_dir/.git" ]; then
    git clone "$repos_dir/canonical.git" "$work_dir"
  else
    git -C "$work_dir" fetch origin
    if [ -n "$(git -C "$work_dir" rev-parse --verify --quiet origin/main)" ]; then
      git -C "$work_dir" reset --hard origin/main
    fi
  fi

  git -C "$work_dir" config user.name memory-synthesizer
  git -C "$work_dir" config user.email memory-synthesizer@local
  git config --global --add safe.directory "$work_dir"
  git config --global --add safe.directory "$repos_dir/canonical.git"
}

with_canonical_lock() {
  mkdir -p "$data_dir/state"
  (
    flock 9
    "$@"
  ) 9>"$data_dir/state/canonical.lock"
}

commit_with_retry() {
  commit_work_dir="$1"
  commit_message="$2"
  commit_pathspec="$3"
  commit_attempt=1
  while [ "$commit_attempt" -le 3 ]; do
    git -C "$commit_work_dir" add -A -- "$commit_pathspec"
    if git -C "$commit_work_dir" commit -m "$commit_message" -- "$commit_pathspec" >/dev/null 2>&1; then
      return 0
    fi
    commit_attempt=$((commit_attempt + 1))
    sleep 2
  done
  return 1
}
