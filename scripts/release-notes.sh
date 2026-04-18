#!/usr/bin/env bash
set -euo pipefail

tag_name="${1:-}"

if [[ -z "${tag_name}" ]]; then
  echo "usage: $0 <tag>" >&2
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
echo "### How to install"
echo
echo "This project is source-only — sing-box is GPLv3, so we do not ship"
echo "pre-built binaries under the MIT license. Build from the attached"
echo "source tarball:"
echo
echo '```bash'
echo "curl -L https://github.com/oglenyaboss/sing-box-agent/archive/refs/tags/${tag_name}.tar.gz | tar xz"
echo "cd sing-box-agent-${tag_name#v}"
echo "make build"
echo '```'
