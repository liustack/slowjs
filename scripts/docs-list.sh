#!/usr/bin/env bash
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# List docs/ with the summary and read_when from each file's front matter.
#
#   scripts/docs-list.sh           print the index
#   scripts/docs-list.sh --check   also fail when a doc has no summary, or when
#                                  a doc has no Chinese version next to it
#
# Front matter format:
#
#   ---
#   summary: 'One line saying what the doc answers'
#   read_when:
#     - Situation in which an agent should read it
#   ---
#
# read_when also accepts the inline form: read_when: [a, b]
#
# Chinese versions (*.zh-CN.md) are not listed. --check fails when a doc of
# docs/ or one of the files in $paired has no Chinese version, or when a
# Chinese version has no English doc.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
docs="$root/docs"
check=false
case "${1:-}" in
  "") ;;
  --check) check=true ;;
  *) echo "usage: $0 [--check]" >&2; exit 2 ;;
esac

if [ ! -d "$docs" ]; then
  echo "no docs/ directory at $root" >&2
  exit 1
fi

# Drafts and history are not part of the index. A *.zh-CN.md file is the
# Chinese version of the English doc next to it, so it is not listed again.
files="$(cd "$root" && find docs -type f -name '*.md' -not -name '*.zh-CN.md' \
  -not -path 'docs/archive/*' -not -path 'docs/research/*' | LC_ALL=C sort)"

missing=0
listed=0
while IFS= read -r file; do
  [ -n "$file" ] || continue
  out="$(awk -v path="$file" '
    function unquote(s) {
      sub(/^[ \t]+/, "", s); sub(/[ \t]+$/, "", s)
      if (s ~ /^'\''.*'\''$/ || s ~ /^".*"$/) s = substr(s, 2, length(s) - 2)
      return s
    }
    NR == 1 { if ($0 != "---") exit; inside = 1; next }
    inside && $0 == "---" { closed = 1; exit }
    inside && /^summary:/ { sub(/^summary:/, ""); summary = unquote($0); list = 0; next }
    inside && /^read_when:/ {
      rest = $0; sub(/^read_when:/, "", rest); rest = unquote(rest)
      if (rest ~ /^\[.*\]$/) {
        rest = substr(rest, 2, length(rest) - 2)
        n = split(rest, items, ",")
        for (i = 1; i <= n; i++) { item = unquote(items[i]); if (item != "") when = when (when == "" ? "" : " | ") item }
        list = 0
      } else list = 1
      next
    }
    inside && list && /^[ \t]+-[ \t]+/ {
      item = $0; sub(/^[ \t]+-[ \t]+/, "", item); item = unquote(item)
      when = when (when == "" ? "" : " | ") item
      next
    }
    inside && /^[^ \t]/ { list = 0 }
    END {
      if (!closed || summary == "") exit 1
      print path " - " summary
      if (when != "") print "  Read when: " when
    }
  ' "$root/$file")" || {
    missing=$((missing + 1))
    echo "warning: $file has no front matter summary" >&2
    continue
  }
  echo "$out"
  listed=$((listed + 1))
done <<EOF
$files
EOF

if [ "$listed" -eq 0 ]; then
  echo "No docs with front matter found in docs/"
fi
echo
echo 'Reminder: before coding, check the "Read when" hints above and read the docs that apply.'

# Every doc is in English and in Chinese (AGENTS.md).
paired="README.md CHANGELOG.md CONTRIBUTING.md SECURITY.md compare/engines/CHANGELOG.md engine/libc/README.md internal/goasm/gen/README.md internal/arewefastyet/README.md"
unpaired=0
if $check; then
  for file in $files $paired; do
    if [ ! -f "$root/${file%.md}.zh-CN.md" ]; then
      echo "warning: $file has no Chinese version ${file%.md}.zh-CN.md" >&2
      unpaired=$((unpaired + 1))
    fi
  done
  zh="$(cd "$root" && find docs -type f -name '*.zh-CN.md' | LC_ALL=C sort)"
  for file in $zh $(for p in $paired; do echo "${p%.md}.zh-CN.md"; done); do
    if [ -f "$root/$file" ] && [ ! -f "$root/${file%.zh-CN.md}.md" ]; then
      echo "warning: $file has no English doc ${file%.zh-CN.md}.md" >&2
      unpaired=$((unpaired + 1))
    fi
  done
fi

if $check && [ $((missing + unpaired)) -gt 0 ]; then
  exit 1
fi
