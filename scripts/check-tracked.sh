#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# check-tracked.sh: fail when git tracks a file that should not be in the
# repository, so that build output, a profile, a log or a key never goes into
# a tag and from there into every download of the module (the Go proxy keeps
# the zip of a version for good). It checks the files of HEAD, or of the ref
# given as $1:
#
#   - a file over 1 MB that the list of large files below does not name
#   - a binary file (as git diff sees it) that the list of binary files below
#     does not match
#   - a name that is never ours to track: a profile, a Python cache, an
#     environment or key file, the GitHub CLI's hosts.yml
#
# A new large or binary file that belongs in the repository goes into its
# list here, in the same commit. scripts/check.sh runs it as the step
# "tracked", and ci.yml runs that step on each push.
set -eu
cd "$(dirname "$0")/.."
ref=${1:-HEAD}

# Files over 1 MB that are meant to be tracked: the C of the engine, the data
# of go-intl, the pinned pages and their copy in compare/engines, and recorded
# test data.
large='
compare/engines/tifa/bundles/shop.js
engine/pgo/pages/shop/shop.js
engine/quickjs.c
engine/test/compare/recorded/jsonstr.txt
internal/gointl/qjsdata/cjdict.bin
internal/gointl/qjsdata/core.bin
internal/gointl/qjsdata/names.bin
internal/gointl/qjsdata/zonenames.bin
temporal/testdata/datetime-format.json
'

# Binary files that are meant to be tracked, as patterns of case.
binary() {
	case $1 in
	assets/*.png | assets/brand/*.png) return 0 ;;
	internal/gointl/qjsdata/*.bin) return 0 ;;
	intl/testdata/chromium/*.json.gz) return 0 ;;
	engine/test/js-fuzz-seeds/*) return 0 ;;
	esac
	return 1
}

failed=0
big=$(git ls-tree -r -l "$ref" | awk '$4 > 1048576 { sub(/^[^\t]*\t/, ""); print }')
for f in $big; do
	printf '%s\n' "$large" | grep -q -x -F "$f" || {
		echo "check-tracked.sh: $f is over 1 MB and not in the list of large files"
		failed=1
	}
done

empty=$(git hash-object -t tree /dev/null)
bins=$(git diff --numstat "$empty" "$ref" | awk -F'\t' '$1 == "-" { print $3 }')
for f in $bins; do
	binary "$f" || {
		echo "check-tracked.sh: $f is a binary file and not in the list of binary files"
		failed=1
	}
done

bad=$(git ls-tree -r --name-only "$ref" | grep -E '\.(profraw|profdata|pprof|pyc|pem|p12|pfx|key)$|(^|/)(__pycache__|\.env|\.env\..*|id_rsa.*|id_ed25519.*|hosts\.yml|\.netrc)(/|$)' || true)
for f in $bad; do
	echo "check-tracked.sh: $f is never to be tracked"
	failed=1
done

[ "$failed" = 0 ] || {
	echo "check-tracked.sh: remove the file with git rm --cached, or add it to its list in scripts/check-tracked.sh"
	exit 1
}
