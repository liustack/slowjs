#!/bin/bash
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# run.sh <out dir> <bin dir> [native dir]: the tifa page runs.
#
# Run from compare/engines, with the engine binaries of
# `go run ./cmd/compare -build-only` in <bin dir>, tifa/polyfill/intl-polyfill.js
# built (tifa/polyfill/build.sh) and, for the reference rows, the engines of
# native/build.sh in [native dir]. All three paths are relative to the caller.
# Linux only: checks and timing both run on CPU 1 under a 4 GiB cgroup cap,
# with TZ=UTC and GOMAXPROCS=1. pagerun.sh retries disturbed processes.
#
#   check   the Intl-free check on slowjs: every page rendered
#           normally and with prelude/nointl-stub.js, and the Intl members
#           each page reaches (prelude/intl-proxy.js, a VM per page)
#   bench   the bench pages (no Intl), 50 warm and 200 timed renders a page
#   pf      the shop pages on the FormatJS polyfill, native Intl removed
#   native  the shop pages on the engine's own Intl, where it has one
#   timer   the time slowjs spends inside Intl, on the polyfill
#           and on its own Intl (prelude/intl-timer.js)
#   mem     memory per VM with and without the polyfill, 20 VMs
#   mui     the same two for the MUI pages (mui-pf, mui-native)
#
# slowjs-tifa is slowjs set up as tifa sets it up (engine/eng_liustacktifa.go):
# its own Intl and Temporal, tifa's Go functions, no polyfill. It runs bench,
# native and mui-native, never pf.
#
# slowjs-tifa-nogoasm is the same row built with slowjs_nogoasm, as tifa
# builds an app with build.goasm off: the interpreter in Go on linux/amd64.
#
# TIFA_ONLY=<engine>,<engine> runs only those engines (names as in the result
# files). The check, timer and mem runs belong to slowjs.
#
# TIFA_BUNDLES=<dir> runs the bundles and inputs of <dir> (an absolute path,
# laid out as this folder: bundles/<app>.js and in/<app>/*.json) in place of
# the pinned ones. A bundle that tifa built as an ES module goes there twice:
# as bundles/<app>.mjs, which the slowjs-tifa rows compile as a module as
# tifa does, and as bundles/<app>.js made by module-script.py, which every
# other row runs as a script.
#
# Results: <out dir>/<engine>-<kind>.json and .meta, HTML in <out dir>/html.
set -e
out=${1:-} bin=${2:-} nat=${3:-}
[ -n "$out" ] && [ -n "$bin" ] || { sed -n '6,32p' "$0"; exit 2; }
# Resolve before changing directory, including output directories not yet made.
absolute() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s/%s\n' "$PWD" "$1" ;; esac; }
out=$(absolute "$out")
bin=$(absolute "$bin")
[ -z "$nat" ] || nat=$(absolute "$nat")
mkdir -p "$out/html" "$out/check"
cd "$(dirname "$0")"
B=${TIFA_BUNDLES:-.}
list() { ls "$B"/in/"$1"/*.json | tr '\n' , | sed 's/,$//'; }
# tifa_bundle <app>: the module form for the tifa rows, when there is one.
tifa_bundle() { if [ -f "$B/bundles/$1.mjs" ]; then echo "$B/bundles/$1.mjs"; else echo "$B/bundles/$1.js"; fi; }
BENCH=$(list bench) SHOP=$(list shop) MUI=$(list mui)
PF=prelude/nointl.js,polyfill/intl-polyfill.js
run() { env TZ=UTC GOMAXPROCS=1 bash pagerun.sh "$@"; }
want() { [ -z "${TIFA_ONLY:-}" ] || case ",$TIFA_ONLY," in *",$1,"*) return 0 ;; *) return 1 ;; esac; }
go_runs() { # go_runs <bin> <engine name> <yes: has own Intl, no: has none, tifa: own Intl and no polyfill run>
  want "$2" || return 0
  local b=("$bin/$1" tifa-ssr -html "$out/html") bench=$B/bundles/bench.js shop=$B/bundles/shop.js mui=$B/bundles/mui.js
  if [ "$3" = tifa ]; then bench=$(tifa_bundle bench) shop=$(tifa_bundle shop) mui=$(tifa_bundle mui); fi
  run "$out/$2-bench" "${b[@]}" -bundle $bench -inputs $BENCH -warm 50 -n 200 -tag -bench
  if [ "$3" != tifa ]; then
    run "$out/$2-pf" "${b[@]}" -bundle $shop -inputs $SHOP -prelude $PF -warm 20 -n 100 -tag -pf
    run "$out/$2-mui-pf" "${b[@]}" -bundle $mui -inputs $MUI -prelude $PF -warm 20 -n 100 -tag -mui-pf
  fi
  if [ "$3" != no ]; then
    run "$out/$2-native" "${b[@]}" -bundle $shop -inputs $SHOP -warm 20 -n 100 -tag -native
    run "$out/$2-mui-native" "${b[@]}" -bundle $mui -inputs $MUI -warm 20 -n 100 -tag -mui-native
  fi
  return 0
}
# Use the same capped entry for functional checks and timing.
if want slowjs; then
for app in bench mui shop; do
  in=$(list $app)
  run "$out/check/$app-normal" "$bin/liustack" tifa-ssr -bundle $B/bundles/$app.js -inputs $in -warm 0 -n 1 -html "$out/check" -tag -$app-normal
  run "$out/check/$app-stub" "$bin/liustack" tifa-ssr -bundle $B/bundles/$app.js -inputs $in -prelude prelude/nointl-stub.js -warm 0 -n 1 -html "$out/check" -tag -$app-stub
  for f in "$B"/in/$app/*.json; do
    run "$out/check/$app-proxy-$(basename "$f" .json)" "$bin/liustack" tifa-ssr -bundle $B/bundles/$app.js -inputs $f -prelude prelude/intl-proxy.js -probe '__intlHits()' -warm 0 -n 0
  done
done
fi
go_runs liustack slowjs yes
go_runs liustacktifa slowjs-tifa tifa
go_runs liustacktifanogoasm slowjs-tifa-nogoasm tifa
if want slowjs; then
run "$out/slowjs-timer-pf" "$bin/liustack" tifa-ssr -bundle $B/bundles/shop.js -inputs $SHOP -prelude $PF,prelude/intl-timer.js -probe '__intlTime()' -warm 20 -n 100
run "$out/slowjs-timer-native" "$bin/liustack" tifa-ssr -bundle $B/bundles/shop.js -inputs $SHOP -prelude prelude/intl-timer.js -probe '__intlTime()' -warm 20 -n 100
for b in liustack:slowjs goquickjs:go-quickjs-interp; do
  run "$out/${b#*:}-mem-pf" "$bin/${b%%:*}" tifa-ssr -bundle $B/bundles/shop.js -inputs $B/in/shop/store-zh-CN-cart.json -prelude $PF -warm 0 -n 0 -mem 20
  run "$out/${b#*:}-mem-native" "$bin/${b%%:*}" tifa-ssr -bundle $B/bundles/shop.js -inputs $B/in/shop/store-zh-CN-cart.json -warm 0 -n 0 -mem 20
done
fi
go_runs goquickjs go-quickjs-interp yes
go_runs goquickjsjit go-quickjs-jit yes
for b in modernc:modernc-quickjs goant:goant-interp goantjit:goant-jit moejs:moejs; do
  go_runs ${b%%:*} ${b#*:} no
done
[ -n "$nat" ] || exit 0
for q in quickjs-2026-06-04 quickjs-master quickjs-ng; do
  want $q || continue
  qjs=$nat/$q/qjs; [ -x "$qjs" ] || qjs=$nat/$q/build/qjs
  d=("$qjs" --std ../native/ssr-driver.js "engine=$q" "html=$out/html")
  run "$out/$q-bench" "${d[@]}" bundle=$B/bundles/bench.js inputs=$BENCH warm=50 n=200 tag=-bench
  run "$out/$q-pf" "${d[@]}" bundle=$B/bundles/shop.js inputs=$SHOP prelude=$PF warm=20 n=100 tag=-pf
  run "$out/$q-mui-pf" "${d[@]}" bundle=$B/bundles/mui.js inputs=$MUI prelude=$PF warm=20 n=100 tag=-mui-pf
done
for m in jit jitless; do
  want node-$m || continue
  f=(); [ "$m" = jitless ] && f=(--jitless)
  d=("$nat/node-v24.11.1-linux-x64/bin/node" "${f[@]}" ../native/node-ssr.cjs "engine=node-$m" "html=$out/html")
  run "$out/node-$m-bench" "${d[@]}" bundle=$B/bundles/bench.js inputs=$BENCH warm=50 n=200 tag=-bench
  run "$out/node-$m-pf" "${d[@]}" bundle=$B/bundles/shop.js inputs=$SHOP prelude=$PF warm=20 n=100 tag=-pf
  run "$out/node-$m-native" "${d[@]}" bundle=$B/bundles/shop.js inputs=$SHOP warm=20 n=100 tag=-native
  run "$out/node-$m-mui-pf" "${d[@]}" bundle=$B/bundles/mui.js inputs=$MUI prelude=$PF warm=20 n=100 tag=-mui-pf
  run "$out/node-$m-mui-native" "${d[@]}" bundle=$B/bundles/mui.js inputs=$MUI warm=20 n=100 tag=-mui-native
done
