#!/bin/sh
# Copyright (c) 2026 The liustack/beastjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# vet-tags.sh: go vet of every Go module of the repository with each build
# tag its files use, for each system the module builds for, so that a file
# behind a tag (a test of testhooks, the hammer, an engine of compare/engines)
# still builds and passes vet, also where it builds for one architecture only
# (the hammer is amd64 only, and a Mac is arm64). go vet builds the test files
# too. The nightly run (.github/workflows/nightly.yml) runs it.
#
# It fails when a Go file uses a build tag that the list below does not have,
# or when a module with Go files is not in it: add the tag or the module there.
# The tags of GOOS, GOARCH, go1.N and the go command's own (unix, cgo, race,
# gc, gccgo, ignore, purego) need no line.
#
# The root module is vetted with the go command as it is called (the oldest
# Go the module supports, in the nightly run). The other modules are not part
# of what users build and may ask for a newer Go in their go.mod, so they run
# with GOTOOLCHAIN=auto, which fetches that Go when the one here is older.
#
# Environment:
#   VET_TAGS_LIST  1: print the list and stop
set -eu
cd "$(dirname "$0")/.."

# The module folder, the -tags of its go vet ("-" for no tags) and the
# systems: all, the six the module runs on, or unix, those but windows
# (compare/engines measures with getrusage, and runs on linux and darwin).
list='
. - all
. testhooks all
. cenginehammer all
. goasmkeep all
. beastjs_anygo all
. gointl_all all
. gointl_walk all
. beastjs_intl_nonames,beastjs_intl_nocjdict all
internal/measure/pages - all
internal/measure/pages pagesspin1 all
internal/measure/pages pagesspin3 all
compare - all
compare/engines eng_liustack unix
compare/engines eng_liustackpolyfill unix
compare/engines eng_moejs unix
compare/engines eng_goquickjs unix
'
unix='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64'
all="$unix windows/amd64 windows/arm64"

if [ "${VET_TAGS_LIST:-}" = 1 ]; then
	echo "$list" | grep -v '^$'
	exit 0
fi

known=$( (go tool dist list | tr '/' '\n'; printf '%s\n' unix cgo race gc gccgo ignore purego) | sort -u)
listed=$(echo "$list" | awk 'NF == 3 && $2 != "-" { print $2 }' | tr ',' '\n' | sort -u)
used=$(git grep -h '^//go:build' -- '*.go' | sed 's,^//go:build,,' | tr -c 'A-Za-z0-9_.\n' '\n' |
	grep -v -e '^$' -e '^go1\.' -e '^goexperiment\.' | sort -u)
missing=$(printf '%s\n' "$used" | grep -v -x -F "$known" | grep -v -x -F "$listed" || true)
if [ -n "$missing" ]; then
	echo "vet-tags.sh: build tags not in the list of scripts/vet-tags.sh:" $missing >&2
	exit 1
fi
mods=$(echo "$list" | awk 'NF == 3 { print $1 }' | sort -u)
for gomod in $(git ls-files '*go.mod'); do
	dir=$(dirname "$gomod")
	git ls-files "$dir/*.go" | grep -q . || continue
	printf '%s\n' "$mods" | grep -q -x -F "$dir" || {
		echo "vet-tags.sh: module $dir is not in the list of scripts/vet-tags.sh" >&2
		exit 1
	}
done

failed=0
while read -r dir tags systems; do
	[ "$tags" = - ] && t= || t=$tags
	case $systems in
	all) targets=$all ;;
	unix) targets=$unix ;;
	*) echo "vet-tags.sh: $dir $tags: systems $systems: want all or unix" >&2; exit 2 ;;
	esac
	[ "$dir" = . ] && toolchain=${GOTOOLCHAIN:-} || toolchain=auto
	for target in $targets; do
		echo "== go vet ${t:+-tags $t }in $dir for $target"
		(cd "$dir" && GOTOOLCHAIN=$toolchain GOOS=${target%/*} GOARCH=${target#*/} go vet ${t:+-tags "$t"} ./...) || failed=1
	done
done <<EOF
$(echo "$list" | grep -v '^$')
EOF
[ "$failed" = 0 ] || { echo "vet-tags.sh: go vet failed"; exit 1; }
