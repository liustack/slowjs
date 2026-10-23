#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# build.sh <dir>: downloads and builds the native reference engines into
# <dir>, each with its own build as shipped: Bellard's QuickJS 2026-06-04
# (the release modernc.org/libquickjs v0.13.1 and so liustack/slowjs are
# translated from) and master at 7d1c13c5d8 with `make qjs` (gcc -O2, no
# LTO), QuickJS-NG v0.17.0 with `make` (CMake Release), and Node v24.11.1
# (linux-x64 binary). CMake 3.31.6 is fetched for QuickJS-NG.
set -e
mkdir -p "$1" && cd "$1"
get() { # get <url> <file> <sha256>
  [ -f "$2" ] || curl -sSfL -o "$2" "$1"
  echo "$3  $2" | sha256sum -c -
}
get https://bellard.org/quickjs/quickjs-2026-06-04.tar.xz quickjs-2026-06-04.tar.xz b376e839b322978313d929fd20663b11ba58b75df5a46c126dd19ea2fa70ad2a
get https://codeload.github.com/bellard/quickjs/tar.gz/7d1c13c5d8 qjs-master.tar.gz 58f176b81fdce49c35ec52f2846cbf664059ff97b289dc08320bc6e299e97862
get https://codeload.github.com/quickjs-ng/quickjs/tar.gz/refs/tags/v0.17.0 ng.tar.gz 559bc4c420475e55c7ab4510adbc562f55d7524d75e8e89d79ce4bb02f5687d9
get https://github.com/Kitware/CMake/releases/download/v3.31.6/cmake-3.31.6-linux-x86_64.tar.gz cmake.tar.gz 5a1133ff103c71eb5120e2cc3de922733e7d8a26a98ae716397e8676adb367bf
get https://nodejs.org/dist/v24.11.1/node-v24.11.1-linux-x64.tar.xz node.tar.xz 60e3b0a8500819514aca603487c254298cd776de0698d3cd08f11dba5b8289a8
tar xf quickjs-2026-06-04.tar.xz
(cd quickjs-2026-06-04 && make -j1 qjs)
mkdir -p quickjs-master && tar xzf qjs-master.tar.gz -C quickjs-master --strip-components=1
(cd quickjs-master && make -j1 qjs)
tar xzf cmake.tar.gz
mkdir -p quickjs-ng && tar xzf ng.tar.gz -C quickjs-ng --strip-components=1
(cd quickjs-ng && PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" make -j1 JOBS=1)
tar xf node.tar.xz
