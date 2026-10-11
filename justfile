set shell := ["bash", "-euo", "pipefail", "-c"]

windmill_url := env("WINDMILL_URL", "http://raspberry.local:8081")
windmill_workspace := env("WINDMILL_WORKSPACE", "memory")

[group('build')]
build dir="./...": gen-mocks
    go build {{ dir }}

[group('test')]
test dir="./...": gen-mocks
    go test -count=1 {{ dir }}

[group('test')]
test-short dir="./...": gen-mocks
    go test -short -count=1 {{ dir }}

[group('quality')]
lint dir="./...": gen-mocks
    golangci-lint run {{ dir }}

[group('quality')]
fmt: gen-mocks
    golangci-lint fmt

[group('quality')]
lint-sh:
    shellcheck test/e2e.sh

[group('test')]
gen-mocks dir="./...":
    #!/usr/bin/env bash
    set -euo pipefail
    pattern="{{ dir }}"
    scope="${pattern%/...}"
    directive="//go:generate go tool moq"
    marker="// mock-source-sum:"

    package_sum() {
        local pkg="$1"
        {
            sha256sum go.mod
            find "${pkg}" -maxdepth 1 -name '*.go' ! -name '*_mock.go' ! -name '*_test.go' -print0 | sort -z | xargs -0 sha256sum
        } | sha256sum | cut -d' ' -f1
    }

    mock_outputs() {
        local pkg="$1"
        grep -h "^${directive}" "${pkg}"/*.go | grep -o -- '-out [^ ]*' | cut -d' ' -f2
    }

    package_status() {
        local pkg="$1"
        local sum="$2"
        local outs
        local out
        local last
        outs=$(mock_outputs "${pkg}")
        while IFS= read -r out; do
            [[ -n "${out}" ]] || continue
            [[ -f "${pkg}/${out}" ]] || { echo stale; return; }
            last=$(tail -n 1 "${pkg}/${out}")
            [[ "${last}" == "${marker} ${sum}" ]] || { echo stale; return; }
        done <<<"${outs}"
        echo fresh
    }

    stamp_mocks() {
        local pkg="$1"
        local sum="$2"
        local outs
        local out
        outs=$(mock_outputs "${pkg}")
        while IFS= read -r out; do
            [[ -n "${out}" ]] || continue
            printf '\n%s %s\n' "${marker}" "${sum}" >>"${pkg}/${out}"
        done <<<"${outs}"
    }

    remove_orphans() {
        local mocks
        local mock
        local has_directive
        mocks=$(find "${scope}" -name '*_mock.go' -not -path './.git/*')
        while IFS= read -r mock; do
            has_directive=0
            grep -qs "^${directive}" "$(dirname "${mock}")"/*.go && has_directive=1
            if [[ "${has_directive}" -eq 0 ]]; then
                rm -f "${mock}"
            fi
        done <<<"${mocks}"
    }

    stale=()
    sums=()
    pkgs=$(grep -rl --include='*.go' "^${directive}" "${scope}" | xargs -r -n1 dirname | sort -u || true)
    while IFS= read -r pkg; do
        [[ -n "${pkg}" ]] || continue
        sum=$(package_sum "${pkg}")
        status=$(package_status "${pkg}" "${sum}")
        if [[ "${status}" == "fresh" ]]; then
            continue
        fi
        stale+=("${pkg}")
        sums+=("${sum}")
    done <<<"${pkgs}"

    remove_orphans

    if [[ "${#stale[@]}" -eq 0 ]]; then
        echo "🧪 Mocks up to date"
        exit 0
    fi

    echo "🧪 Regenerating mocks in ${#stale[@]} package(s)..."
    for pkg in "${stale[@]}"; do
        find "${pkg}" -maxdepth 1 -name '*_mock.go' -delete
    done
    go generate "${stale[@]}"
    for i in "${!stale[@]}"; do
        stamp_mocks "${stale[${i}]}" "${sums[${i}]}"
    done

[group('test')]
gen-mocks-force: && gen-mocks
    find . -name '*_mock.go' -not -path './.git/*' -delete

[group('windmill')]
windmill-failed-count:
    #!/usr/bin/env bash
    set -euo pipefail
    base="{{ windmill_url }}/api/w/{{ windmill_workspace }}"
    token=$(~/.claude/skills/secrets/bin/get-secret windmill)

    api_get() {
        local path="$1"
        curl -fsS -H "Authorization: Bearer ${token}" "${base}${path}"
    }

    total=$(api_get "/jobs/completed/list?success=false&per_page=10000" | jq 'length')
    echo "failed total: ${total}"
    echo "count  canceled  path"
    api_get "/jobs/completed/list?success=false&per_page=10000" |
        jq -r 'group_by([(.script_path // .job_kind), .canceled])
            | map({n: length, c: .[0].canceled, p: (.[0].script_path // .[0].job_kind)})
            | sort_by(-.n)[]
            | "\(.n)  \(.c)  \(.p)"'

[confirm("Delete ALL failed Windmill runs and their logs? This is irreversible.")]
[group('windmill')]
windmill-delete-failed batch="500":
    #!/usr/bin/env bash
    set -euo pipefail
    base="{{ windmill_url }}/api/w/{{ windmill_workspace }}"
    batch="{{ batch }}"
    token=$(~/.claude/skills/secrets/bin/get-secret windmill)

    first_page() {
        curl -fsS -H "Authorization: Bearer ${token}" \
            "${base}/jobs/completed/list?success=false&per_page=${batch}" | jq '[.[].id]'
    }

    delete_ids() {
        local ids="$1"
        curl -fsS -X POST -H "Authorization: Bearer ${token}" \
            -H "Content-Type: application/json" -d "${ids}" "${base}/jobs/delete"
    }

    total=0
    previous_first=""
    while true; do
        ids=$(first_page)
        count=$(jq 'length' <<<"${ids}")
        if [[ "${count}" -eq 0 ]]; then
            echo "done, total deleted: ${total}"
            exit 0
        fi
        first=$(jq -r '.[0]' <<<"${ids}")
        if [[ "${first}" == "${previous_first}" ]]; then
            echo "no progress: job ${first} still listed after delete" >&2
            exit 1
        fi
        previous_first="${first}"
        delete_ids "${ids}"
        echo
        total=$((total + count))
        echo "deleted batch of ${count}, running total: ${total}"
    done
