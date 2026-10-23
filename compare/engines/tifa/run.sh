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
#   check   the Intl-free check on liustack-quickjs: every page rendered
#           normally and with prelude/nointl-stub.js, and the Intl members
#           each page reaches (prelude/intl-proxy.js, a VM per page)
#   bench   the bench pages (no Intl), 50 warm and 200 timed renders a page
#   pf      the shop pages on the FormatJS polyfill, native Intl removed
#   native  the shop pages on the engine's own Intl, where it has one
#   timer   the time liustack-quickjs spends inside Intl, on the polyfill
#           and on its own Intl (prelude/intl-timer.js)
#   mem     memory per VM with and without the polyfill, 20 VMs
#
# Results: <out dir>/<engine>-<kind>.json and .meta, HTML in <out dir>/html.
set -e
out=${1:-} bin=${2:-} nat=${3:-}
[ -n "$out" ] && [ -n "$bin" ] || { sed -n '6,24p' "$0"; exit 2; }
# Resolve before changing directory, including output directories not yet made.
absolute() { case "$1" in /*) printf '%s\n' "$1" ;; *) printf '%s/%s\n' "$PWD" "$1" ;; esac; }
out=$(absolute "$out")
bin=$(absolute "$bin")
[ -z "$nat" ] || nat=$(absolute "$nat")
mkdir -p "$out/html" "$out/check"
cd "$(dirname "$0")"
list() { ls in/"$1"/*.json | tr '\n' , | sed 's/,$//'; }
BENCH=$(list bench) SHOP=$(list shop)
PF=prelude/nointl.js,polyfill/intl-polyfill.js
run() { env TZ=UTC GOMAXPROCS=1 bash pagerun.sh "$@"; }
go_runs() { # go_runs <bin> <engine name> <has own Intl>
  local b=("$bin/$1" tifa-ssr -html "$out/html")
  run "$out/$2-bench" "${b[@]}" -bundle bundles/bench.js -inputs $BENCH -warm 50 -n 200 -tag -bench
  run "$out/$2-pf" "${b[@]}" -bundle bundles/shop.js -inputs $SHOP -prelude $PF -warm 20 -n 100 -tag -pf
  [ "$3" = yes ] && run "$out/$2-native" "${b[@]}" -bundle bundles/shop.js -inputs $SHOP -warm 20 -n 100 -tag -native
  return 0
}
# Use the same capped entry for functional checks and timing.
for app in bench mui shop; do
  in=$(list $app)
  run "$out/check/$app-normal" "$bin/liustack" tifa-ssr -bundle bundles/$app.js -inputs $in -warm 0 -n 1 -html "$out/check" -tag -$app-normal
  run "$out/check/$app-stub" "$bin/liustack" tifa-ssr -bundle bundles/$app.js -inputs $in -prelude prelude/nointl-stub.js -warm 0 -n 1 -html "$out/check" -tag -$app-stub
  for f in in/$app/*.json; do
    run "$out/check/$app-proxy-$(basename "$f" .json)" "$bin/liustack" tifa-ssr -bundle bundles/$app.js -inputs $f -prelude prelude/intl-proxy.js -probe '__intlHits()' -warm 0 -n 0
  done
done
go_runs liustack liustack-quickjs yes
run "$out/liustack-quickjs-timer-pf" "$bin/liustack" tifa-ssr -bundle bundles/shop.js -inputs $SHOP -prelude $PF,prelude/intl-timer.js -probe '__intlTime()' -warm 20 -n 100
run "$out/liustack-quickjs-timer-native" "$bin/liustack" tifa-ssr -bundle bundles/shop.js -inputs $SHOP -prelude prelude/intl-timer.js -probe '__intlTime()' -warm 20 -n 100
for b in liustack:liustack-quickjs goquickjs:go-quickjs-interp; do
  run "$out/${b#*:}-mem-pf" "$bin/${b%%:*}" tifa-ssr -bundle bundles/shop.js -inputs in/shop/store-zh-CN-cart.json -prelude $PF -warm 0 -n 0 -mem 20
  run "$out/${b#*:}-mem-native" "$bin/${b%%:*}" tifa-ssr -bundle bundles/shop.js -inputs in/shop/store-zh-CN-cart.json -warm 0 -n 0 -mem 20
done
go_runs goquickjs go-quickjs-interp yes
go_runs goquickjsjit go-quickjs-jit yes
for b in modernc:modernc-quickjs goant:goant-interp goantjit:goant-jit moejs:moejs; do
  go_runs ${b%%:*} ${b#*:} no
done
[ -n "$nat" ] || exit 0
for q in quickjs-2026-06-04 quickjs-master quickjs-ng; do
  qjs=$nat/$q/qjs; [ -x "$qjs" ] || qjs=$nat/$q/build/qjs
  d=("$qjs" --std ../native/ssr-driver.js "engine=$q" "html=$out/html")
  run "$out/$q-bench" "${d[@]}" bundle=bundles/bench.js inputs=$BENCH warm=50 n=200 tag=-bench
  run "$out/$q-pf" "${d[@]}" bundle=bundles/shop.js inputs=$SHOP prelude=$PF warm=20 n=100 tag=-pf
done
for m in jit jitless; do
  f=(); [ "$m" = jitless ] && f=(--jitless)
  d=("$nat/node-v24.11.1-linux-x64/bin/node" "${f[@]}" ../native/node-ssr.cjs "engine=node-$m" "html=$out/html")
  run "$out/node-$m-bench" "${d[@]}" bundle=bundles/bench.js inputs=$BENCH warm=50 n=200 tag=-bench
  run "$out/node-$m-pf" "${d[@]}" bundle=bundles/shop.js inputs=$SHOP prelude=$PF warm=20 n=100 tag=-pf
  run "$out/node-$m-native" "${d[@]}" bundle=bundles/shop.js inputs=$SHOP warm=20 n=100 tag=-native
done
