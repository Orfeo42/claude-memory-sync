#!/bin/sh
if [ -z "$CLAUDE_CODE_OAUTH_TOKEN" ]; then
  echo "CLAUDE_CODE_OAUTH_TOKEN is required (claude setup-token)" >&2
  exit 1
fi

if [ -n "$ANTHROPIC_API_KEY" ]; then
  echo "ANTHROPIC_API_KEY must not be set: it would override subscription OAuth auth" >&2
  exit 1
fi

script_dir=$(dirname "$0")

if [ "${INTAKE_RUN_ONCE:-false}" = "true" ]; then
  if "$script_dir/intake.sh"; then
    echo "intake pass completed"
    exit 0
  else
    echo "intake pass failed" >&2
    exit 1
  fi
fi

while true; do
  if "$script_dir/intake.sh"; then
    echo "intake pass completed"
  else
    echo "intake pass failed" >&2
  fi
  sleep "${INTAKE_INTERVAL:-45m}"
done
