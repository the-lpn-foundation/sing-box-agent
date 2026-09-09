#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PRE_COMMIT_CONFIG="$ROOT_DIR/.pre-commit-config.yaml"
PRE_COMMIT_HOOK="$ROOT_DIR/.git/hooks/pre-commit"

failures=0

check() {
  local name="$1"
  local message="$2"
  if [[ "$3" -eq 0 ]]; then
    echo "hook_check:result=ok check=$name message=$message"
  else
    echo "hook_check:result=fail check=$name message=$message"
    failures=$((failures + 1))
  fi
}

if [[ -f "$PRE_COMMIT_CONFIG" ]]; then
  check "pre_commit_config" "found .pre-commit-config.yaml" 0
else
  check "pre_commit_config" "missing .pre-commit-config.yaml" 1
fi

for required in "id: gofumpt-check" "id: golangci-lint" "id: gitleaks" "id: go-vet"; do
  if grep -q "$required" "$PRE_COMMIT_CONFIG" 2>/dev/null; then
    check "config_$required" "configured $required" 0
  else
    check "config_$required" "missing $required in .pre-commit-config.yaml" 1
  fi
done

if command -v pre-commit >/dev/null 2>&1; then
  check "pre_commit_binary" "pre-commit is installed" 0
else
  check "pre_commit_binary" "pre-commit is not installed; remediation: pipx install pre-commit or pip install pre-commit" 1
fi

if [[ -x "$PRE_COMMIT_HOOK" ]]; then
  check "pre_commit_hook" ".git/hooks/pre-commit exists and is executable" 0
else
  check "pre_commit_hook" "pre-commit hook is not installed; remediation: pre-commit install" 1
fi

if [[ "$failures" -eq 0 ]]; then
  echo "hook_check:status=pass"
  exit 0
fi

echo "hook_check:status=fail failure_count=$failures"
echo "hook_check:remediation run pre-commit install, then re-run scripts/hook-check.sh"
exit 1
