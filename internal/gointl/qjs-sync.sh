#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.

# qjs-sync.sh keeps internal/gointl a copy of go-intl
# (https://github.com/go-quickjs/go-intl) at the commit in UPSTREAM, with our
# changes on top.
#
# What is ours: every file or directory whose name starts with "qjs" (this
# script, qjs.patch, qjs_*.go, qjsdata/, internal/qjspack/, the qjs_walk.go of
# each data package), and UPSTREAM. Everything else is go-intl's, rewritten
# from github.com/go-quickjs/go-intl to github.com/liustack/quickjs/internal/gointl
# and changed only by qjs.patch, which is kept to the data loading layer and
# to notices: source.go stops embedding data.pack and serves our packs
# instead, internal/blob reports the pool parts it reads when the pack tool
# walks the data, dictbe.go takes a missing break dictionary for none, so
# that a build without the CJK dictionary still segments, and the files
# ported from calendrical_calculations (temporal/solar.go,
# temporal/zoneinfo.go) say so in a header, as its Apache-2.0 license asks.
#
# Run it from anywhere: it works on the module the script is in.
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
#	qjs-sync.sh test       run go-intl's own tests, with ours, in a scratch module,
#	                       on packs of every language (arguments go to go test)
#	qjs-sync.sh test-chromium
#	                       the same on packs of Chromium's languages, with the
#	                       cases of the other languages taken out of go-intl's
#	                       corpora
#	qjs-sync.sh chromium   record in qjsdata/chromium.json what Chromium resolves
#	                       go-intl's available locales to (needs Node and
#	                       Playwright's Chromium, see qjs-chromium.cjs)
#	qjs-sync.sh patch      write qjs.patch again from the changes to go-intl's files
#
# Moving to a new release: set VERSION and COMMIT in UPSTREAM, then checkout,
# update (fix and rerun "patch" where qjs.patch no longer applies), chromium,
# pack, test, test-chromium, and run the module's tests. If a data package
# changed what its Decode reads lazily, the pack tool fails its check that
# every pool part is reachable: change that package's qjs_walk.go to read it.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
. "$here/UPSTREAM"
upstream=${GOINTL_UPSTREAM:-$HOME/.cache/quickjs-gointl/go-intl}
ours=github.com/liustack/quickjs/internal/gointl

# chromium_skips are go-intl's tests that do not pass on the packs of
# Chromium's languages, for what they are: they name in their code a
# language the packs leave out or keep the plural rules of only (my, lo,
# ps, ast, wo, kea, iw), or count the locales Node has, or count the cases
# of the corpora test-chromium cuts.
chromium_skips='TestWeekYearIsTheExtendedYears|TestCompactCurrency|TestCompactLongOwnPatterns|TestCurrencyFormats|TestCurrencyUnitPatterns|TestListContextualJoins|TestLocaleMatcherLocales|TestCorpusLocalesParse|TestEveryCaseParses|TestCaseCounts'

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
# rewritten. With tests, it copies their tests, the packages they import and
# testdata as well.
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
	unformatted=$(cd "$root" && gofmt -l internal/gointl)
	[ -z "$unformatted" ] || die "gofmt lists files: $unformatted"
	(cd "$root" && go build ./internal/gointl/...)
	echo "internal/gointl is $VERSION with qjs.patch"
	;;
pack)
	need_checkout
	shift
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$upstream/data" -out internal/gointl/qjsdata "$@")
	;;
test | test-chromium)
	need_checkout
	which=$1
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
	# Two tests read go-intl's data/ and data.pack, which quickjs does not
	# carry: pack_test.go, which does not build without data.pack, and
	# TestGeneratedTablesAreWellFormed.
	rm -f "$mod/pack_test.go"
	skip='TestGeneratedTablesAreWellFormed'
	if [ "$which" = test ]; then
		# go-intl's tests hold it to Node in every language ICU has, so
		# they run on packs of every language: what they check is that the
		# packs serve go-intl's data. The tests of Chromium's locales are
		# left out.
		(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
			-data "$upstream/data" -out "$mod/qjsdata" -locales all >/dev/null)
		skip="$skip|TestQJSAvailable|TestQJSResolveMatchesChromium"
	else
		# On the packs built in, with the corpus cases of what they leave
		# out taken out (see chromium_skips for the tests left out).
		(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
			-data "$upstream/data" -out "$mod/qjsdata" -cut-corpus "$mod/testdata" >/dev/null)
		skip="$skip|$chromium_skips"
	fi
	printf 'module github.com/liustack/quickjs\n\ngo 1.24\n' >"$tmp/go.mod"
	(cd "$tmp" && go test -skip "^($skip)\$" "$@" ./internal/gointl/...)
	;;
chromium)
	need_checkout
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$upstream/data" -tags >"$tmp/tags")
	node "$here/qjs-chromium.cjs" "$tmp/tags"
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
