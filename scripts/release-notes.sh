#!/usr/bin/env bash
set -euo pipefail

tag_name="${1:-}"
checksums_file="${2:-dist/checksums.txt}"

if [[ -z "${tag_name}" ]]; then
  echo "usage: $0 <tag> [checksums-file]" >&2
  exit 1
fi

previous_tag=""
if previous_tag=$(git describe --tags --abbrev=0 "${tag_name}^" 2>/dev/null); then
  commit_range="${previous_tag}..${tag_name}"
else
  commit_range="${tag_name}"
fi

changelog=""
changelog="$(git log --pretty='- %s (%h)' "${commit_range}" 2>/dev/null || true)"

echo "## Release ${tag_name}"
echo
echo "### Changelog"
if [[ -n "${changelog}" ]]; then
  echo "${changelog}"
else
  echo "- No commit entries found for this release range."
fi
echo
echo "### Checksums"
if [[ -f "${checksums_file}" ]]; then
  echo '```text'
  cat "${checksums_file}"
  echo '```'
else
  echo "- Checksums file not found at ${checksums_file}."
fi
