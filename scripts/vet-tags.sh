#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# vet-tags.sh: go vet of every Go module of the repository with each build
# tag its files use, so that a file behind a tag (a test of testhooks, the
# hammer, an engine of compare/engines) still builds and passes vet. go vet
# builds the test files too. The nightly run (.github/workflows/nightly.yml)
# runs it.
#
# It fails when a Go file uses a build tag that the list below does not have,
# or when a module with Go files is not in it: add the tag or the module there.
# The tags of GOOS, GOARCH, go1.N and the go command's own (unix, cgo, race,
# gc, gccgo, ignore, purego) need no line.
#
# Environment:
#   VET_TAGS_LIST  1: print the list and stop
set -eu
cd "$(dirname "$0")/.."

# The module folder and the -tags of one go vet, "-" for no tags.
list='
. -
. testhooks
. cenginehammer
. goasmkeep
. slowjs_anygo
. gointl_all
. gointl_walk
. slowjs_intl_nonames,slowjs_intl_nocjdict
internal/measure/pages -
internal/measure/pages pagesspin1
internal/measure/pages pagesspin3
compare -
compare/engines eng_liustack
compare/engines eng_liustackpolyfill
compare/engines eng_moejs
compare/engines eng_goquickjs
'

if [ "${VET_TAGS_LIST:-}" = 1 ]; then
	echo "$list" | grep -v '^$'
	exit 0
fi

known=$( (go tool dist list | tr '/' '\n'; printf '%s\n' unix cgo race gc gccgo ignore purego) | sort -u)
listed=$(echo "$list" | awk 'NF == 2 && $2 != "-" { print $2 }' | tr ',' '\n' | sort -u)
used=$(git grep -h '^//go:build' -- '*.go' | sed 's,^//go:build,,' | tr -c 'A-Za-z0-9_.\n' '\n' |
	grep -v -e '^$' -e '^go1\.' -e '^goexperiment\.' | sort -u)
missing=$(printf '%s\n' "$used" | grep -v -x -F "$known" | grep -v -x -F "$listed" || true)
if [ -n "$missing" ]; then
	echo "vet-tags.sh: build tags not in the list of scripts/vet-tags.sh:" $missing >&2
	exit 1
fi
mods=$(echo "$list" | awk 'NF == 2 { print $1 }' | sort -u)
for gomod in $(git ls-files '*go.mod'); do
	dir=$(dirname "$gomod")
	git ls-files "$dir/*.go" | grep -q . || continue
	printf '%s\n' "$mods" | grep -q -x -F "$dir" || {
		echo "vet-tags.sh: module $dir is not in the list of scripts/vet-tags.sh" >&2
		exit 1
	}
done

failed=0
while read -r dir tags; do
	[ "$tags" = - ] && t= || t=$tags
	echo "== go vet ${t:+-tags $t }in $dir"
	(cd "$dir" && go vet ${t:+-tags "$t"} ./...) || failed=1
done <<EOF
$(echo "$list" | grep -v '^$')
EOF
[ "$failed" = 0 ] || { echo "vet-tags.sh: go vet failed"; exit 1; }
