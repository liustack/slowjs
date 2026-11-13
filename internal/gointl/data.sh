#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.

# data.sh makes the data internal/gointl builds in, qjsdata/, from the data
# of the go-intl release in UPSTREAM (https://github.com/go-quickjs/go-intl).
#
# The code of internal/gointl was copied from go-intl v0.6.1 once, and is
# ours: it is changed here, and nothing copies go-intl's code again. Only
# go-intl's data/ directory is read, as data. The decoders of that data are
# ours too, so a go-intl release that changes a data format needs the same
# change made by hand in the decoder here: pack fails when it cannot read a
# part, and its check that every pool part is reachable fails when a data
# package reads a part lazily that its qjs_walk.go does not walk.
#
# Run it from anywhere: it works on the module the script is in.
#
#	data.sh checkout       clone go-intl at UPSTREAM's commit into $GOINTL_UPSTREAM
#	                       (default: $HOME/.cache/quickjs-gointl/go-intl), and
#	                       make its data again next to it with the time zones
#	                       of the update qjstz names (see tz_data)
#	data.sh pack           rebuild qjsdata/ from the checkout's data/, with
#	                       qjstz's time zones
#	                       (extra arguments go to the pack tool, see
#	                       internal/qjspack/gen)
#	data.sh test           run the tests that need every language
#	                       (build tag gointl_all, the tests that came with
#	                       go-intl's code), with the others, in a scratch copy
#	                       whose packs have every language of go-intl's data
#	                       and its own time zones (arguments go to go test)
#	data.sh test-chromium  run the tests and recorded Chromium corpora on the
#	                       packs built in (arguments go to go test)
#	data.sh chromium       record in qjsdata/chromium.json what Chromium resolves
#	                       go-intl's available locales to (needs Node and
#	                       Playwright's Chromium, see qjs-chromium.cjs)
#
# Moving to the data of a new go-intl release: set VERSION and COMMIT in
# UPSTREAM, then checkout, chromium, pack, test, test-chromium, and run the
# module's tests.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
. "$here/UPSTREAM"
. "$here/qjstz"
upstream=${GOINTL_UPSTREAM:-$HOME/.cache/quickjs-gointl/go-intl}
cache=$(dirname "$upstream")
# tzdata is a copy of the checkout whose data/ has the time zones of
# qjstz's update, which the pack tool reads (see tz_data).
tzdata=$upstream-tz$TZ_VERSION
tzstamp="$COMMIT $TZ_VERSION $TZ_ZONEINFO64 $TZ_TIMEZONETYPES $TZ_WINDOWSZONES $TZ_METAZONES"

die() {
	echo "data.sh: $*" >&2
	exit 1
}

need_checkout() {
	[ -d "$upstream/.git" ] || die "no checkout at $upstream: run checkout first"
	got=$(git -C "$upstream" rev-parse HEAD)
	[ "$got" = "$COMMIT" ] || die "$upstream is at $got, not $COMMIT: run checkout"
	[ "$(cat "$tzdata/qjstz.stamp" 2>/dev/null)" = "$tzstamp" ] ||
		die "$tzdata is not the data of $COMMIT with tz $TZ_VERSION: run checkout"
}

sha256() {
	{ sha256sum "$1" 2>/dev/null || shasum -a 256 "$1"; } | cut -d' ' -f1
}

# fetch downloads $1 to $2 unless $2 is there already, and checks that its
# sha256 is $3.
fetch() {
	if [ ! -f "$2" ] || [ "$(sha256 "$2")" != "$3" ]; then
		curl -fsSL -o "$2.tmp" "$1" || die "cannot download $1"
		mv "$2.tmp" "$2"
	fi
	got=$(sha256 "$2")
	[ "$got" = "$3" ] || die "$1 has sha256 $got, want $3"
}

# tz_data makes $tzdata: the checkout, without its history and data.pack,
# with data/tz/, data/windowszones.bin and data/abbreviationzones.bin
# written again by go-intl's own tzgen from ICU's time zone update
# TZ_VERSION, and data/tzversion.bin saying which update that is. tzgen
# checks the files against the checksums of go-intl's icusrc, which are set
# to qjstz's in the copy. Nothing else of go-intl's data changes, so its
# zone names and metazones stay those of the update go-intl was made from.
tz_data() {
	dl=$cache/icu-tz-$TZ_VERSION
	mkdir -p "$dl"
	base=https://raw.githubusercontent.com/unicode-org/icu-data/main/tzdata/icunew/$TZ_VERSION/44
	fetch "$base/zoneinfo64.txt" "$dl/zoneinfo64.txt" "$TZ_ZONEINFO64"
	fetch "$base/timezoneTypes.txt" "$dl/timezoneTypes.txt" "$TZ_TIMEZONETYPES"
	fetch "$base/windowsZones.txt" "$dl/windowsZones.txt" "$TZ_WINDOWSZONES"
	fetch "$base/metaZones.txt" "$dl/metaZones.txt" "$TZ_METAZONES"
	fetch "$ICU_SOURCES_URL" "$cache/$ICU_SOURCES" "$ICU_SOURCES_SHA256"
	grep -q "TZVersion { \"$TZ_VERSION\" }" "$dl/zoneinfo64.txt" ||
		die "$dl/zoneinfo64.txt is not tz $TZ_VERSION"
	rm -rf "$tzdata"
	mkdir -p "$tzdata"
	(cd "$upstream" && git archive --format=tar HEAD) | (cd "$tzdata" && tar xf - --exclude=data.pack)
	icusrc=$tzdata/internal/icusrc/icusrc.go
	for f in zoneinfo64:$TZ_ZONEINFO64 timezoneTypes:$TZ_TIMEZONETYPES windowsZones:$TZ_WINDOWSZONES metaZones:$TZ_METAZONES; do
		perl -pi -e "s/(\"${f%%:*}\":\\s*)\"[0-9a-f]{64}\"/\$1\"${f#*:}\"/" "$icusrc"
		grep -q "${f#*:}" "$icusrc" || die "cannot set the checksum of ${f%%:*} in $icusrc"
	done
	(cd "$tzdata" && go run ./internal/tzgen "$dl" "$cache/$ICU_SOURCES")
	printf '%s' "$TZ_VERSION" >"$tzdata/data/tzversion.bin"
	echo "$tzstamp" >"$tzdata/qjstz.stamp"
}


case "${1:-}" in
checkout)
	if [ ! -d "$upstream/.git" ]; then
		mkdir -p "$(dirname "$upstream")"
		git clone --quiet "$REPO" "$upstream"
	fi
	git -C "$upstream" fetch --quiet --tags origin
	git -C "$upstream" -c advice.detachedHead=false checkout --quiet "$COMMIT"
	if [ "$(cat "$tzdata/qjstz.stamp" 2>/dev/null)" != "$tzstamp" ]; then
		tz_data
	fi
	need_checkout
	echo "$upstream is at $VERSION ($COMMIT), and $tzdata has its data with tz $TZ_VERSION"
	;;
pack)
	need_checkout
	shift
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$tzdata/data" -out internal/gointl/qjsdata "$@")
	;;
test)
	need_checkout
	shift
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	mod=$tmp/internal/gointl
	mkdir -p "$mod"
	(cd "$here" && tar cf - --exclude=qjsdata .) | (cd "$mod" && tar xf -)
	mkdir -p "$mod/qjsdata"
	cp "$here"/qjsdata/NOTICE-* "$mod/qjsdata/"
	# Their corpora are data of the release, as its data/ is: Node's answers
	# on that data.
	cp -R "$upstream/testdata" "$mod/testdata"
	# The tests that came with go-intl's code hold it to Node in every
	# language ICU has, so they run on packs of every language of go-intl's
	# data as it is, with its own time zones, not qjstz's: what they check
	# is that the packs serve that data. The tests of Chromium's locales, and
	# of the tz release qjstz names, are left out, and so is one that reads
	# go-intl's data/ directory, which is not here.
	gotz=$(sed -n 's/^| IANA tzdb | \([0-9a-z]*\),.*/\1/p' "$here/SOURCES.md")
	[ -n "$gotz" ] || die "cannot read the tz release of go-intl's data from $here/SOURCES.md"
	data=$tmp/gointl-data
	cp -R "$upstream/data" "$data"
	printf '%s' "$gotz" >"$data/tzversion.bin"
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$data" -out "$mod/qjsdata" -locales all >/dev/null)
	skip='TestGeneratedTablesAreWellFormed|TestQJSAvailable|TestQJSResolveMatchesChromium|TestQJSTZVersion'
	printf 'module github.com/liustack/quickjs\n\ngo %s\n' "$(sed -n 's/^go //p' "$root/go.mod")" >"$tmp/go.mod"
	(cd "$tmp" && go test -tags gointl_all -skip "^($skip)\$" "$@" ./internal/gointl/...)
	;;
test-chromium)
	shift
	# These packs select Chromium's ICU resources, not Node's full data.
	# Keep Node's corpus in test and check the shipped packs against the
	# recorded browser answers, including their exact known differences.
	(cd "$root" && go test "$@" ./internal/gointl/... ./intl/... ./temporal/...)
	;;
chromium)
	need_checkout
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	(cd "$root" && go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen \
		-data "$tzdata/data" -tags >"$tmp/tags")
	node "$here/qjs-chromium.cjs" "$tmp/tags"
	;;
*)
	sed -n '/^# data.sh makes the data/,/^set -eu/p' "$0" | sed '$d; s/^# \{0,1\}//'
	exit 2
	;;
esac
