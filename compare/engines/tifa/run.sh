#!/bin/bash
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# run.sh <out dir> <bin dir> <native dir>: the tifa page runs.
#
# Run from compare/engines, with the engine binaries of
# `go run ./cmd/compare -build-only` in <bin dir>, the polyfills built
# (tifa/polyfill/build.sh) and the engines of native/build.sh in <native dir>.
# All three paths are relative to the caller. Linux only: every run is on one
# CPU (TIFA_CPU, default 1) under a 4 GiB cgroup cap, with TZ=UTC and
# GOMAXPROCS=1. pagerun.sh retries disturbed processes.
#
# Every engine runs the bundles as they come, with its own Intl and URL where
# it has them, and tifa's Go functions are registered in none. A polyfill is
# loaded only into an engine that lacks the feature: the FormatJS polyfills
# (prelude/nointl.js first, which removes a partial Intl) for Intl, core-js
# (polyfill/corejs-polyfill.js) for URL, URLSearchParams, structuredClone and
# the iterator helpers. Which engine gets which is the lists below.
# liustack-slowjs-polyfill is liustack-slowjs without the packages intl, temporal and url (the
# bare engine), so it gets the polyfills of an engine that lacks them: it is run on the pf kinds.
#
#   bench   the bench pages (they need no Intl), 50 warm and 200 timed renders
#   native  the shop pages on the engine's own Intl    (-native), or
#   pf      on the FormatJS polyfill when it has none  (-pf)
#   mui     the same two for the MUI pages             (mui-native, mui-pf)
#
# TIFA_ONLY=<engine>,<engine> runs only those engines (names as in the result
# files), which cuts a run into pieces on a shared machine.
#
# TIFA_BUNDLES=<dir> runs the bundles and inputs of <dir> (an absolute path,
# laid out as this folder: bundles/<app>.js and in/<app>/*.json) in place of
# the pinned ones.
#
# Results: <out dir>/<engine>-<kind>.json and .meta, HTML in <out dir>/html.
set -e
out=${1:-} bin=${2:-} nat=${3:-}
[ -n "$out" ] && [ -n "$bin" ] && [ -n "$nat" ] || { sed -n '6,36p' "$0"; exit 2; }
# Resolve before changing directory, including output directories not yet made.
absolute() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s/%s\n' "$PWD" "$1" ;; esac; }
out=$(absolute "$out")
bin=$(absolute "$bin")
nat=$(absolute "$nat")
mkdir -p "$out/html"
cd "$(dirname "$0")"
B=${TIFA_BUNDLES:-.}
list() { ls "$B"/in/"$1"/*.json | tr '\n' , | sed 's/,$//'; }
BENCH=$(list bench) SHOP=$(list shop) MUI=$(list mui)
INTL=prelude/nointl.js,polyfill/intl-polyfill.js
CORE=polyfill/corejs-polyfill.js
bad=0
# run <out stem> <command...>. A run in which no page rendered is not an error of the run (an
# engine that cannot load a bundle is a result), but it is said at the end and makes the exit status 1.
run() {
  env TZ=UTC GOMAXPROCS=1 bash pagerun.sh "$@"
  if [ -f "$1.json" ] && ! grep -q '"ok":true' "$1.json"; then
    echo "run.sh: no page rendered in $1.json: $(head -c 300 "$1.json")" >&2
    bad=1
  fi
}
want() { [ -z "${TIFA_ONLY:-}" ] || case ",$TIFA_ONLY," in *",$1,"*) return 0 ;; *) return 1 ;; esac; }
# go_runs <bin> <engine name> <polyfill for URL: "" or the prelude>
# The Go engines of these three calls have an Intl of their own (a partial one for moejs).
go_runs() {
  want "$2" || return 0
  local b=("$bin/$1" tifa-ssr -html "$out/html") pre=() p=$3
  [ -z "$p" ] || pre=(-prelude "$p")
  run "$out/$2-bench" "${b[@]}" -bundle $B/bundles/bench.js -inputs $BENCH "${pre[@]}" -warm 50 -n 200 -tag -bench
  run "$out/$2-native" "${b[@]}" -bundle $B/bundles/shop.js -inputs $SHOP "${pre[@]}" -warm 20 -n 100 -tag -native
  run "$out/$2-mui-native" "${b[@]}" -bundle $B/bundles/mui.js -inputs $MUI "${pre[@]}" -warm 20 -n 100 -tag -mui-native
}
go_runs liustack liustack-slowjs ""            # its own Intl, URL and Temporal (packages intl, temporal, url)
# go_pf_runs <bin> <engine name>: a Go engine with no Intl, URL or Temporal of its own,
# set up as the native engines are: the FormatJS polyfills for the shop and MUI pages, core-js for the rest.
go_pf_runs() {
  want "$2" || return 0
  local b=("$bin/$1" tifa-ssr -html "$out/html")
  run "$out/$2-bench" "${b[@]}" -bundle $B/bundles/bench.js -inputs $BENCH -prelude $CORE -warm 50 -n 200 -tag -bench
  run "$out/$2-pf" "${b[@]}" -bundle $B/bundles/shop.js -inputs $SHOP -prelude $INTL,$CORE -warm 20 -n 100 -tag -pf
  run "$out/$2-mui-pf" "${b[@]}" -bundle $B/bundles/mui.js -inputs $MUI -prelude $INTL,$CORE -warm 20 -n 100 -tag -mui-pf
}
go_pf_runs liustackpolyfill liustack-slowjs-polyfill   # the same engine as the line above, with none of those packages
go_runs goquickjs go-quickjs-interp $CORE
go_runs moejs moejs ../native/moejs-shim.js,$CORE   # moejs lacks FinalizationRegistry: native/moejs-shim.js
# native_runs <engine name> <driver command...>: no Intl of their own, so the
# FormatJS polyfills for the shop and MUI pages.
native_runs() {
  local e=$1; shift
  want "$e" || return 0
  local d=("$@" "engine=$e" "html=$out/html")
  run "$out/$e-bench" "${d[@]}" bundle=$B/bundles/bench.js inputs=$BENCH prelude=$CORE warm=50 n=200 tag=-bench
  run "$out/$e-pf" "${d[@]}" bundle=$B/bundles/shop.js inputs=$SHOP prelude=$INTL,$CORE warm=20 n=100 tag=-pf
  run "$out/$e-mui-pf" "${d[@]}" bundle=$B/bundles/mui.js inputs=$MUI prelude=$INTL,$CORE warm=20 n=100 tag=-mui-pf
}
qjs=$nat/quickjs-2026-06-04/qjs
native_runs quickjs-2026-06-04 "$qjs" --std ../native/ssr-driver.js
native_runs hermes bash ../native/hermes-run.sh hbc "$nat/hermes" "$out/hbc"
# Node has its own Intl and URL, and lacks Iterator.concat.
if want node-jitless; then
  d=("$nat/node-v24.22.3-linux-x64/bin/node" --jitless ../native/node-ssr.cjs "engine=node-jitless" "html=$out/html")
  run "$out/node-jitless-bench" "${d[@]}" bundle=$B/bundles/bench.js inputs=$BENCH prelude=$CORE warm=50 n=200 tag=-bench
  run "$out/node-jitless-native" "${d[@]}" bundle=$B/bundles/shop.js inputs=$SHOP prelude=$CORE warm=20 n=100 tag=-native
  run "$out/node-jitless-mui-native" "${d[@]}" bundle=$B/bundles/mui.js inputs=$MUI prelude=$CORE warm=20 n=100 tag=-mui-native
fi
[ "$bad" = 0 ] || { echo "run.sh: some runs rendered no page (see above)" >&2; exit 1; }
