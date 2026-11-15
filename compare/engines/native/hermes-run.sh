#!/bin/bash
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# hermes-run.sh <src|hbc> <hermes dir> <bytecode dir> name=value ...: one run of
# native/hermes-ssr.js on the Hermes of native/build.sh (<hermes dir>/build-rel),
# with the arguments of ssr-driver.js. In mode hbc, every file named by
# prelude= and bundle= is first compiled by `hermesc -O` into <bytecode dir>
# when it is not there yet or is older than its source (outside the timed run).
# Prints what the other native rows print, and writes each page's HTML to the
# file named by html= (the Hermes shell cannot write a file).
set -e
mode=$1 hd=$2 bd=$3; shift 3
here=$(cd "$(dirname "$0")" && pwd)
args=()
for a in "$@"; do
  case $a in
  prelude=*|bundle=*)
    k=${a%%=*} v=${a#*=} list=()
    IFS=, read -ra fs <<<"$v"
    [ "$k" = prelude ] && fs=("$here/hermes-shim.js" "${fs[@]}")
    for f in "${fs[@]}"; do
      if [ "$k" = bundle ]; then
        # Hermes V1 has no async generators: lower them and for-await to ES2017 async functions, for Hermes only.
        mkdir -p "$bd"
        low=$bd/$(basename "$f")
        if [ ! -f "$low" ] || [ "$f" -nt "$low" ]; then
          "$here/../tifa/polyfill/node_modules/.bin/esbuild" "$f" --supported:async-generator=false --supported:for-await=false --legal-comments=none --outfile="$low" >/dev/null
        fi
        f=$low
      fi
      if [ "$mode" = hbc ]; then
        o=$bd/$(basename "$f").hbc
        mkdir -p "$bd"
        if [ ! -f "$o" ] || [ "$f" -nt "$o" ]; then
          "$hd/build-rel/bin/hermesc" -O -w -Xes6-block-scoping -emit-binary -out "$o" "$f" >/dev/null 2>&1
        fi
      fi
      list+=("$f")
    done
    args+=("$k=$(IFS=,; echo "${list[*]}")") ;;
  *) args+=("$a") ;;
  esac
done
"$hd/build-rel/bin/hermes" -Xhermes-internal-test-methods "$here/hermes-ssr.js" -- mode="$mode" hbc="$bd" driver="$here/ssr-driver.js" "${args[@]}" |
  while IFS= read -r line; do
    case $line in
    "@html "*) rest=${line#@html }; printf '%b' "${rest#* }" > "${rest%% *}" ;;
    *) printf '%s\n' "$line" ;;
    esac
  done
