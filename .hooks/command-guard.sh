#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DENY_LIST_PATH="${COMMAND_GUARD_DENY_LIST:-$SCRIPT_DIR/deny-list.txt}"
ALLOW_ONCE_TOKEN="I_ACKNOWLEDGE_DESTRUCTIVE_COMMAND_RISK"

command_input="${COMMAND_GUARD_COMMAND:-${*:-}}"

if [[ -z "$command_input" ]]; then
  echo "command_guard:error missing command input"
  echo "command_guard:remediation pass command text as arguments or set COMMAND_GUARD_COMMAND"
  exit 2
fi

if [[ ! -f "$DENY_LIST_PATH" ]]; then
  echo "command_guard:error deny-list file not found at $DENY_LIST_PATH"
  echo "command_guard:remediation ensure .hooks/deny-list.txt exists or set COMMAND_GUARD_DENY_LIST"
  exit 2
fi

matched_pattern=""
while IFS= read -r pattern || [[ -n "$pattern" ]]; do
  [[ -z "$pattern" || "$pattern" =~ ^[[:space:]]*# ]] && continue
  if printf '%s\n' "$command_input" | grep -E -q "$pattern"; then
    matched_pattern="$pattern"
    break
  fi
done <"$DENY_LIST_PATH"

if [[ -n "$matched_pattern" ]]; then
  if [[ "${COMMAND_GUARD_ALLOW_ONCE:-}" == "$ALLOW_ONCE_TOKEN" ]]; then
    echo "command_guard:event allow_once"
    echo "command_guard:details command matched deny-list pattern but explicit acknowledgment token accepted"
    echo "command_guard:json={\"status\":\"allowed_once\",\"command\":\"$command_input\",\"pattern\":\"$matched_pattern\"}"
    exit 0
  fi

  echo "command_guard:event blocked"
  echo "command_guard:reason destructive command matched deny-list"
  echo "command_guard:command $command_input"
  echo "command_guard:pattern $matched_pattern"
  echo "command_guard:json={\"status\":\"blocked\",\"command\":\"$command_input\",\"pattern\":\"$matched_pattern\"}"
  echo "command_guard:remediation command denied by policy (block-by-default)."
  echo "command_guard:remediation if you intentionally need this once, re-run with:"
  echo "command_guard:remediation COMMAND_GUARD_ALLOW_ONCE=$ALLOW_ONCE_TOKEN ./.hooks/command-guard.sh \"$command_input\""
  echo "command_guard:remediation do not make allow-once part of routine workflow."
  exit 1
fi

echo "command_guard:event allowed"
echo "command_guard:json={\"status\":\"allowed\",\"command\":\"$command_input\"}"
exit 0
