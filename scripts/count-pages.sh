#!/usr/bin/env bash
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# count-pages.sh: the instructions a render of each page takes in two builds,
# counted by callgrind in the window of internal/measure/pages (-cg), and the
# HTML of each page compared between the two. .github/workflows/count.yml runs
# it, and it runs the same on any linux/amd64 machine with valgrind and Go.
# See docs/measuring.md.
#
#   count-pages.sh build BASE_TREE NEW_TREE BIN
#       the formal binaries of internal/measure/pages, BIN/base@0.0 from
#       BASE_TREE and BIN/new@0.0 from NEW_TREE (go tool measure bench build
#       -positions 0.0, run in NEW_TREE)
#   count-pages.sh count BIN REPO OUT GROUP
#       count the pages of GROUP with both binaries and write OUT/GROUP.tsv
#       (kind, page, the count of the window of each side, the renders in it,
#       the SHA-256 of the first HTML of each side). The pages come from the
#       checkout REPO. Exits 1 when a page renders other HTML in the two builds.
#   count-pages.sh table TSV...
#       the Markdown table of the TSV files: each page, the sum of each kind
#
# GROUP is wb, store-en, store-zh, mui, react (the pages of internal/measure/pages)
# or v8 (each suite of the V8 benchmarks of internal/arewefastyet/v8-v7 as a
# bundle whose render runs every benchmark of the suite once).
#
# Environment:
#   COUNT_JOBS   processes at once (default: the number of CPUs)
#   COUNT_WARM   renders of a page before the window (default 2)
#   COUNT_TIMED  renders of a page in the window (default 3)
#   COUNT_ONLY   comma list: only these pages (or V8 suites) of the group
#   COUNT_GOGC   GOGC of the runs under callgrind (default off: with 100, Go's
#                collector runs in the window of one run and not of the next,
#                and the same binary moves by up to 0.3%)
set -euo pipefail

gogc=${COUNT_GOGC:-off}
warm=${COUNT_WARM:-2}
timed=${COUNT_TIMED:-3}
v8suites="Richards DeltaBlue Crypto RayTrace EarleyBoyer RegExp Splay NavierStokes"

die() {
	echo "count-pages.sh: $*" >&2
	exit 2
}

# pages GROUP REPO: the kind of the group and its pages, one per line, kind first
pages() {
	local kind glob
	case $1 in
	wb) kind=wb glob='shop/in/wb-*.json' ;;
	store-en) kind=store glob='shop/in/store-en-US-*.json' ;;
	store-zh) kind=store glob='shop/in/store-zh-CN-*.json' ;;
	mui) kind=mui glob='mui/in/*.json' ;;
	react)
		printf 'react\nreact-ssr\n'
		return
		;;
	v8)
		echo v8
		printf '%s\n' $v8suites
		return
		;;
	*) die "no group $1: wb, store-en, store-zh, mui, react or v8" ;;
	esac
	echo $kind
	local f found=0
	for f in "$2"/engine/pgo/pages/$glob; do
		[ -f "$f" ] || continue
		found=1
		basename "$f" .json
	done
	[ $found = 1 ] || die "no page $glob in $2/engine/pgo/pages"
}

# v8bundle SUITE REPO FILE: base.js, the file of the suite and render(), which
# runs every benchmark of the suite once. The benchmarks check their own results
# and throw when one is wrong.
v8bundle() {
	local f
	case $1 in
	Richards) f=richards ;; DeltaBlue) f=deltablue ;; Crypto) f=crypto ;;
	RayTrace) f=raytrace ;; EarleyBoyer) f=earley-boyer ;; RegExp) f=regexp ;;
	Splay) f=splay ;; NavierStokes) f=navier-stokes ;;
	*) die "no V8 suite $1" ;;
	esac
	local d=$2/internal/arewefastyet/v8-v7
	{
		cat "$d/base.js" "$d/$f.js"
		cat <<'EOF'
var countSuite = BenchmarkSuite.suites[0];
for (var countI = 0; countI < countSuite.benchmarks.length; countI++) countSuite.benchmarks[countI].Setup();
function render() {
  for (var i = 0; i < countSuite.benchmarks.length; i++) countSuite.benchmarks[i].run();
  return "<" + countSuite.name + ">";
}
EOF
	} >"$3"
}

# args KIND PAGE OUT: the arguments of the pages program for one page
args() {
	if [ "$1" = v8 ]; then
		echo "-bundle $3/v8-$2.js -react -limit 0"
	else
		[ "$1" = react ] && echo "-kind react" || echo "-kind $1 -pages $2"
	fi
}

# one BIN REPO OUT KIND SIDE PAGE: one page with one binary under callgrind
one() {
	local bin=$1 repo=$2 out=$3 kind=$4 side=$5 page=$6
	local o=$out/raw/$side-$page
	rm -f "$o".cg "$o".cg.*
	# shellcheck disable=SC2046
	if ! (cd "$out/raw" && GOMAXPROCS=1 GODEBUG=asyncpreemptoff=1 GOGC=$gogc valgrind --tool=callgrind --vgdb=no \
		--callgrind-out-file="$o.cg" --dump-instr=no --compress-strings=no --compress-pos=no \
		"$bin/$side@0.0" -test.run '^$' -test.bench Pages -test.benchtime 1x -repo "$repo" \
		$(args "$kind" "$page" "$out") -cg -warm "$warm" -timed "$timed" -evaltimeout 1h) >"$o.log" 2>&1; then
		tail -n 30 "$o.log" >&2
		die "$side $page: the run under callgrind failed (log $o.log)"
	fi
	local dumps=("$o".cg.*)
	[ ${#dumps[@]} = 1 ] && [ -f "${dumps[0]}" ] || die "$side $page: want one dump of the window, found ${dumps[*]}"
	# the totals line, and the own instructions of the functions of the engine
	# (internal/cengine). The cost line after calls= is the cost of the call,
	# not the caller's own.
	local ir
	ir=$(awk '/^fn=/ { e = index($0, "/internal/cengine.") > 0; next }
		/^calls=/ { skip = 1; next }
		/^[0-9]/ { if (!skip && e) s += $2; skip = 0; next }
		{ skip = 0 }
		/^totals:/ { t = $2 }
		END { printf "%d %d", t, s }' "${dumps[0]}")
	[ "${ir%% *}" -gt 0 ] || die "$side $page: no totals in ${dumps[0]}"
	echo "$ir" >"$o.ir"
	echo "$side $page: ${ir% *} instructions in the window, ${ir#* } of them in the engine"
}

# htmlhash BIN REPO OUT KIND SIDE PAGE: the SHA-256 of the first HTML of one page, run natively
htmlhash() {
	local bin=$1 repo=$2 out=$3 kind=$4 side=$5 page=$6
	local o=$out/raw/$side-$page
	# shellcheck disable=SC2046
	if ! (cd "$out/raw" && GOMAXPROCS=1 "$bin/$side@0.0" -test.run '^$' -test.bench Pages -test.benchtime 1x \
		-repo "$repo" $(args "$kind" "$page" "$out") -warm 0 -timed 1 -hash -evaltimeout 1h) >"$o.hash.log" 2>&1; then
		tail -n 30 "$o.hash.log" >&2
		die "$side $page: the native run failed"
	fi
	local h
	h=$(awk '$1 == "html" { print $3; exit }' "$o.hash.log")
	[ -n "$h" ] || die "$side $page: no html line"
	echo "$h" >"$o.hash"
}

count() {
	[ $# = 4 ] || die "count BIN REPO OUT GROUP"
	local bin repo out group=$4
	bin=$(cd "$1" && pwd)
	repo=$(cd "$2" && pwd)
	mkdir -p "$3/raw"
	out=$(cd "$3" && pwd)
	command -v valgrind >/dev/null || die "no valgrind"
	local list kind
	mapfile -t list < <(pages "$group" "$repo")
	kind=${list[0]}
	list=("${list[@]:1}")
	if [ -n "${COUNT_ONLY:-}" ]; then
		local keep=() p
		for p in "${list[@]}"; do
			case ",$COUNT_ONLY," in *",$p,"*) keep+=("$p") ;; esac
		done
		list=("${keep[@]}")
		[ ${#list[@]} -gt 0 ] || die "COUNT_ONLY=$COUNT_ONLY names no page of $group"
	fi
	local p s
	if [ "$kind" = v8 ]; then
		for p in "${list[@]}"; do v8bundle "$p" "$repo" "$out/v8-$p.js"; done
	fi
	echo "count $group ($kind): ${list[*]}, $warm renders before the window and $timed in it"
	local self
	self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
	for p in "${list[@]}"; do
		for s in base new; do echo "$s $p"; done
	done | xargs -P "${COUNT_JOBS:-$(nproc)}" -n 2 "$self" one "$bin" "$repo" "$out" "$kind"
	for p in "${list[@]}"; do
		for s in base new; do echo "$s $p"; done
	done | xargs -P "${COUNT_JOBS:-$(nproc)}" -n 2 "$self" htmlhash "$bin" "$repo" "$out" "$kind"
	local tsv=$out/$group.tsv differ=0
	: >"$tsv"
	for p in "${list[@]}"; do
		local bh nh
		bh=$(cat "$out/raw/base-$p.hash")
		nh=$(cat "$out/raw/new-$p.hash")
		[ "$bh" = "$nh" ] || {
			echo "count-pages.sh: $p renders other HTML: base $bh, new $nh" >&2
			differ=1
		}
		local bi ni
		read -r -a bi <"$out/raw/base-$p.ir"
		read -r -a ni <"$out/raw/new-$p.ir"
		printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$kind" "$p" "${bi[0]}" "${ni[0]}" "$timed" "$bh" "$nh" \
			"${bi[1]}" "${ni[1]}" >>"$tsv"
	done
	table "$tsv"
	return $differ
}

# table TSV...: one row per page and one per kind, the counts a render
table() {
	[ $# -gt 0 ] || die "table TSV..."
	cat "$@" | sort -t "$(printf '\t')" -k1,1 -k2,2 | awk -F '\t' '
		function pct(a, b) { return a > 0 ? sprintf("%+.2f%%", 100 * (b - a) / a) : "-" }
		function sum(k) {
			printf "| **%s** | **sum** | **%.0f** | **%.0f** | **%s** | **%.0f** | **%.0f** | **%s** | |\n", k,
				sb[k], sn[k], pct(sb[k], sn[k]), eb[k], en[k], pct(eb[k], en[k])
		}
		BEGIN {
			print "| kind | page | base, instructions a render | new | change | base, in the engine | new, in the engine | change in the engine | same HTML |"
			print "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |"
		}
		{
			if (NR > 1 && $1 != kind) sum(kind)
			kind = $1
			b = $3 / $5; n = $4 / $5; be = $8 / $5; ne = $9 / $5
			sb[kind] += b; sn[kind] += n; eb[kind] += be; en[kind] += ne
			printf "| %s | %s | %.0f | %.0f | %s | %.0f | %.0f | %s | %s |\n", $1, $2, b, n, pct(b, n), be, ne, pct(be, ne),
				($6 == $7 ? "yes" : "**no**")
		}
		END { if (NR > 0) sum(kind) }'
}

build() {
	[ $# = 3 ] || die "build BASE_TREE NEW_TREE BIN"
	local base new bin
	base=$(cd "$1" && pwd)
	new=$(cd "$2" && pwd)
	mkdir -p "$3"
	bin=$(cd "$3" && pwd)
	cd "$new"
	go tool measure bench build -side base -tree "$base" -app "$base/internal/measure/pages" -pkg . -test \
		-positions 0.0 -out "$bin"
	go tool measure bench build -side new -tree "$new" -app "$new/internal/measure/pages" -pkg . -test \
		-positions 0.0 -out "$bin"
	sha256sum "$bin/base@0.0" "$bin/new@0.0"
}

[ $# -gt 0 ] || die "build, count or table (see the top of the script)"
cmd=$1
shift
case $cmd in
build) build "$@" ;;
count) count "$@" ;;
table) table "$@" ;;
one) one "$@" ;;
htmlhash) htmlhash "$@" ;;
*) die "no command $cmd" ;;
esac
