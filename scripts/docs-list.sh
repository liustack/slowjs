#!/usr/bin/env bash
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# List docs/ with the summary and read_when from each file's front matter.
#
#   scripts/docs-list.sh           print the index
#   scripts/docs-list.sh --check   also fail when a doc has no summary, when
#                                  README.md or README.zh-CN.md is missing, or
#                                  when a user doc and its Chinese version do
#                                  not match (below)
#
# Front matter format:
#
#   ---
#   summary: 'One line saying what the doc answers'
#   audience: user
#   read_when:
#     - Situation in which an agent should read it
#   ---
#
# read_when also accepts the inline form: read_when: [a, b]
#
# audience: user marks a doc for people who use the module or the slowjs
# command. It is the only value, and a doc without it is English only. A user
# doc docs/x.md has a Chinese version docs/x.zh-CN.md, changed in the same
# commit, whose front matter has summary and read_when in Chinese and
#
#   translatedFrom: <the first 12 hex digits of the SHA-256 of docs/x.md>
#
# --check fails when a user doc has no Chinese version, when translatedFrom is
# not the hash of the English file as it is now (the warning gives the hash),
# when a *.zh-CN.md has no summary, and when a *.zh-CN.md is not next to a user
# doc. The index lists the English docs only. README.md and README.zh-CN.md
# have their own rule (AGENTS.md).
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

# entry FILE: the lines of FILE in the index, or status 1 when its front matter
# has no summary.
entry() {
  awk -v path="$1" '
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
  ' "$root/$1"
}

# field FILE KEY: the value of the key KEY in the front matter of FILE, or
# nothing.
field() {
  awk -v key="$2" '
    NR == 1 { if ($0 != "---") exit; next }
    $0 == "---" { exit }
    index($0, key ":") == 1 {
      v = substr($0, length(key) + 2); sub(/^[ \t]+/, "", v); sub(/[ \t]+$/, "", v)
      if (v ~ /^'\''.*'\''$/ || v ~ /^".*"$/) v = substr(v, 2, length(v) - 2)
      print v; exit
    }
  ' "$root/$1"
}

missing=0
listed=0
while IFS= read -r file; do
  [ -n "$file" ] || continue
  out="$(entry "$file")" || {
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

$check || exit 0

unpaired=0
for file in README.md README.zh-CN.md; do
  if [ ! -f "$root/$file" ]; then
    echo "warning: $file is missing" >&2
    unpaired=$((unpaired + 1))
  fi
done

# sha FILE: the first 12 hex digits of the SHA-256 of FILE. sha256sum is on
# Linux and in Git Bash, shasum on macOS.
if command -v sha256sum > /dev/null 2>&1; then
  sha() { sha256sum < "$root/$1" | cut -c1-12; }
elif command -v shasum > /dev/null 2>&1; then
  sha() { shasum -a 256 < "$root/$1" | cut -c1-12; }
else
  echo "docs-list.sh: --check needs sha256sum or shasum" >&2
  exit 1
fi

# Each user doc has a Chinese version translated from it as it is now.
while IFS= read -r file; do
  [ -n "$file" ] || continue
  audience="$(field "$file" audience)"
  case "$audience" in
    "") continue ;;
    user) ;;
    *)
      echo "warning: $file has audience: $audience, and the only audience is user" >&2
      unpaired=$((unpaired + 1))
      continue ;;
  esac
  zh="${file%.md}.zh-CN.md"
  if [ ! -f "$root/$zh" ]; then
    echo "warning: $file is a user doc and has no Chinese version $zh" >&2
    unpaired=$((unpaired + 1))
    continue
  fi
  want="$(sha "$file")"
  from="$(field "$zh" translatedFrom)"
  if [ -z "$from" ]; then
    echo "warning: $zh has no translatedFrom: $want (the hash of $file)" >&2
    unpaired=$((unpaired + 1))
  elif [ "$from" != "$want" ]; then
    echo "warning: $zh is translated from another version of $file (translatedFrom: $from, now $want): change the Chinese to match and set translatedFrom: $want" >&2
    unpaired=$((unpaired + 1))
  fi
done <<EOF
$files
EOF

# A Chinese version is only next to a user doc, and has a summary of its own.
zhs="$(cd "$root" && find docs -type f -name '*.zh-CN.md' | LC_ALL=C sort)"
while IFS= read -r zh; do
  [ -n "$zh" ] || continue
  en="${zh%.zh-CN.md}.md"
  if [ ! -f "$root/$en" ] || [ "$(field "$en" audience)" != user ]; then
    echo "warning: $zh is next to no user doc: only a doc with audience: user in its front matter has a Chinese version ($en)" >&2
    unpaired=$((unpaired + 1))
  fi
  if ! entry "$zh" > /dev/null; then
    echo "warning: $zh has no front matter summary" >&2
    unpaired=$((unpaired + 1))
  fi
done <<EOF
$zhs
EOF

if [ $((missing + unpaired)) -gt 0 ]; then
  exit 1
fi
