#!/bin/sh
# Copyright 2026 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# qjs-sync.sh keeps internal/gointl a copy of go-intl
# (https://github.com/go-quickjs/go-intl) at the commit in UPSTREAM, with our
# changes on top.
#
# What is ours: every file or directory whose name starts with "qjs" (this
# script, qjs.patch, qjs_*.go, qjsdata/, internal/qjspack/, the qjs_walk.go of
# each data package), and UPSTREAM. Everything else is go-intl's, rewritten
# from github.com/go-quickjs/go-intl to github.com/liustack/quickjs/internal/gointl
# and changed only by qjs.patch, which is kept to the data loading layer:
# source.go stops embedding data.pack and serves our packs instead, and
# internal/blob reports the pool parts it reads when the pack tool walks the
# data.
#
# Run it from anywhere; it works on the module the script is in.
#
#	qjs-sync.sh checkout   clone go-intl at UPSTREAM's commit into $GOINTL_UPSTREAM
#	                       (default: $HOME/.cache/quickjs-gointl/go-intl)
#	qjs-sync.sh update     replace go-intl's files here with the checkout's runtime
#	                       sources (the packages the root, date and temporal
#	                       import, without tests), rewrite the imports and apply
#	                       qjs.patch
#	qjs-sync.sh pack       rebuild qjsdata/ from the checkout's data/
#	                       (extra arguments go to the pack tool, see
#	                       internal/qjspack/gen)
#	qjs-sync.sh test       run go-intl's own tests against qjsdata/, in a scratch
#	                       module (arguments go to go test)
#	qjs-sync.sh patch      write qjs.patch again from the changes to go-intl's files
#
# Moving to a new release: set VERSION and COMMIT in UPSTREAM, then checkout,
# update (fix and rerun "patch" where qjs.patch no longer applies), pack, test,
# and run the module's tests. If a data package changed what its Decode reads
# lazily, the pack tool fails its check that every pool part is reachable;
# change that package's qjs_walk.go to read it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
. "$here/UPSTREAM"
upstream=${GOINTL_UPSTREAM:-$HOME/.cache/quickjs-gointl/go-intl}
ours=github.com/liustack/quickjs/internal/gointl

die() {
	echo "qjs-sync.sh: $*" >&2
	exit 1
}

need_checkout() {
	[ -d "$upstream/.git" ] || die "no checkout at $upstream: run checkout first"
	got=$(git -C "$upstream" rev-parse HEAD)
	[ "$got" = "$COMMIT" ] || die "$upstream is at $got, not $COMMIT: run checkout"
}

# runtime_dirs prints the package directories, relative to the checkout,
# that the root, date and temporal packages are built from.
runtime_dirs() {
	(cd "$upstream" && go list -f '{{.Dir}}' -deps . ./date ./temporal) |
		while read -r d; do
			case "$d" in
			"$upstream") echo . ;;
			"$upstream"/*) echo "${d#"$upstream"/}" ;;
			esac
		done
}

# rewrite changes go-intl's import path to ours in the Go files under $1.
rewrite() {
	find "$1" -name '*.go' -exec perl -pi -e "s#\Q$MODULE\E(?=[/\"])#$ours#g" {} +
}

# copy_upstream copies go-intl's runtime files from the checkout into $1,
# rewritten; with tests, their tests, the packages they import and testdata
# as well.
copy_upstream() {
	dst=$1
	tests=${2:-}
	for d in $(runtime_dirs); do
		mkdir -p "$dst/$d"
		for f in "$upstream/$d"/*.go; do
			case "$f" in
			*_test.go) [ -n "$tests" ] || continue ;;
			esac
			cp "$f" "$dst/$d/"
		done
	done
	cp "$upstream/LICENSE" "$upstream/SOURCES.md" "$dst/"
	if [ -n "$tests" ]; then
		for d in $(cd "$upstream" && go list -f '{{.Dir}}' -deps -test . ./date ./temporal | sort -u); do
			case "$d" in
			"$upstream"/internal/*)
				rel=${d#"$upstream"/}
				[ -d "$dst/$rel" ] && continue
				mkdir -p "$dst/$rel"
				cp "$d"/*.go "$dst/$rel/"
				;;
			esac
		done
		cp -R "$upstream/testdata" "$dst/testdata"
	fi
	rewrite "$dst"
}

case "${1:-}" in
checkout)
	if [ ! -d "$upstream/.git" ]; then
		mkdir -p "$(dirname "$upstream")"
		git clone --quiet "$REPO" "$upstream"
	fi
	git -C "$upstream" fetch --quiet --tags origin
	git -C "$upstream" -c advice.detachedHead=false checkout --quiet "$COMMIT"
	need_checkout
	echo "$upstream is at $VERSION ($COMMIT)"
	;;
update)
	need_checkout
	# Remove go-intl's files, keeping ours.
	find "$here" -mindepth 1 \( -name 'qjs*' -o -name UPSTREAM \) -prune -o -type f -print |
		while read -r f; do rm -f "$f"; done
	find "$here" -mindepth 1 -type d -empty -delete
	copy_upstream "$here"
	(cd "$root" && git apply --directory=internal/gointl "$here/qjs.patch") ||
		die "qjs.patch does not apply: fix the files by hand, then run patch"
	(cd "$root" && gofmt -l internal/gointl && go build ./internal/gointl/...)
	echo "internal/gointl is $VERSION with qjs.patch"
	;;
pack)
	need_checkout
	shift
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$upstream/data" -out internal/gointl/qjsdata "$@")
	;;
test)
	need_checkout
	shift
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	mod=$tmp/internal/gointl
	copy_upstream "$mod" tests
	# Our files and changes on top, then go-intl's tests and testdata.
	(cd "$here" && find . -name 'qjs*' -prune -print) | while read -r f; do
		mkdir -p "$mod/$(dirname "$f")"
		cp -R "$here/$f" "$mod/$f"
	done
	(cd "$tmp" && git init --quiet && git apply --directory=internal/gointl "$here/qjs.patch")
	# pack_test.go checks data.pack against data/, neither of which is here.
	rm -f "$mod/pack_test.go"
	printf 'module github.com/liustack/quickjs\n\ngo 1.24\n' >"$tmp/go.mod"
	(cd "$tmp" && go test "$@" ./internal/gointl/...)
	;;
patch)
	need_checkout
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	copy_upstream "$tmp/a"
	mkdir -p "$tmp/b"
	(cd "$here" && find . \( -name 'qjs*' -o -name UPSTREAM \) -prune -o -type f -print) |
		while read -r f; do
			mkdir -p "$tmp/b/$(dirname "$f")"
			cp "$here/$f" "$tmp/b/$f"
		done
	# Without prefixes the files are named a/x and b/x, which git apply
	# takes as x.
	(cd "$tmp" && git diff --no-index --no-prefix a b >"$here/qjs.patch") || [ $? -eq 1 ]
	echo "wrote $here/qjs.patch"
	;;
*)
	sed -n '/^# qjs-sync.sh keeps/,/^set -eu/p' "$0" | sed '$d; s/^# \{0,1\}//'
	exit 2
	;;
esac
