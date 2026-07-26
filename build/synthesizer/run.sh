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

while true; do
  if "$script_dir/synth.sh"; then
    echo "synthesis pass completed"
  else
    echo "synthesis pass failed" >&2
  fi
  sleep "${SYNTH_INTERVAL:-24h}"
done
