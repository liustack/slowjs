#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
# build.sh <dir>: downloads and builds the native reference engines into
# <dir>, each with its own build as shipped: Bellard's QuickJS 2026-06-04
# (the release liustack/quickjs is built from) and master at e585640ffa with `make qjs` (gcc -O2, no
# LTO, and no -DNDEBUG so asserts run), QuickJS-NG v0.18.0 with `make` (CMake Release), and Node v24.22.3
# (linux-x64 binary), and Hermes V1 release 260318099.0.0-stable built Release with clang. CMake 3.31.6 is fetched for QuickJS-NG and Hermes.
set -e
mkdir -p "$1" && cd "$1"
get() { # get <url> <file> <sha256>
  [ -f "$2" ] || curl -sSfL -o "$2" "$1"
  echo "$3  $2" | sha256sum -c -
}
get https://bellard.org/quickjs/quickjs-2026-06-04.tar.xz quickjs-2026-06-04.tar.xz b376e839b322978313d929fd20663b11ba58b75df5a46c126dd19ea2fa70ad2a
get https://codeload.github.com/bellard/quickjs/tar.gz/e585640ffa qjs-master.tar.gz 4f793dc8d210815888ccb451a3e7547eb3b53a8e14b4c6ef09ab23be94e30832
get https://codeload.github.com/quickjs-ng/quickjs/tar.gz/refs/tags/v0.18.0 ng.tar.gz 28bcd89aa3683fd2a946537e5ce3f11cc46b5cbf3383c7561aaef3df5ff9f1dd
get https://github.com/Kitware/CMake/releases/download/v3.31.6/cmake-3.31.6-linux-x86_64.tar.gz cmake.tar.gz 5a1133ff103c71eb5120e2cc3de922733e7d8a26a98ae716397e8676adb367bf
get https://nodejs.org/dist/v24.22.3/node-v24.22.3-linux-x64.tar.xz node.tar.xz 4f2b175a6d53a5c617a80dba2676238a0688cef28c6da4eba35c7d844e66fcfb
tar xf quickjs-2026-06-04.tar.xz
(cd quickjs-2026-06-04 && make -j1 qjs)
mkdir -p quickjs-master && tar xzf qjs-master.tar.gz -C quickjs-master --strip-components=1
(cd quickjs-master && make -j1 qjs)
tar xzf cmake.tar.gz
mkdir -p quickjs-ng && tar xzf ng.tar.gz -C quickjs-ng --strip-components=1
(cd quickjs-ng && PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" make -j1 JOBS=1)
tar xf node.tar.xz
# Hermes V1 release 260318099.0.0-stable (fd22091391c3), interpreter only: the
# release has no x86-64 JIT, and HERMESVM_ALLOW_JIT=0 keeps the option off.
# Needs clang and a C++ toolchain. Release build, Intl off, as the V8 suite
# session of 2026-11-14 built it.
get https://codeload.github.com/facebook/hermes/tar.gz/fd22091391c38ea0111fdd45bd2693d2f50bfc6f hermes.tar.gz 8ec236ddcaadd6b7612d1da1b7d4abd9afc6b68cac461b6cd6e75fd1bb705442
mkdir -p hermes && tar xzf hermes.tar.gz -C hermes --strip-components=1
(cd hermes && PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" cmake -S . -B build-rel -G "Unix Makefiles" -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DHERMES_ENABLE_INTL=OFF -DHERMESVM_ALLOW_JIT=0 &&
  PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" cmake --build build-rel --target hermes hermesc -j 4)
