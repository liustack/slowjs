#!/bin/sh
# Copyright (c) 2026 The liustack/beastjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
# build.sh <dir>: downloads and builds the native reference engines into
# <dir>, each with its own build as shipped: Bellard's QuickJS 2026-06-04
# (the release liustack/beastjs is built from) and master at e585640ffa with `make qjs` (gcc -O2, no
# LTO, and no -DNDEBUG so asserts run), QuickJS-NG v0.18.0 with `make` (CMake Release), and Node v26.13.1
# (linux-x64 binary), and Hermes V1 release 260318099.0.7 built Release with clang. CMake 3.31.6 is fetched for QuickJS-NG and Hermes.
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
get https://nodejs.org/dist/v26.13.1/node-v26.13.1-linux-x64.tar.xz node.tar.xz 5c063e46bab0fbc1c17cc445d7ff129cbdcd0471be6ed83b6ab864f8143bc864
tar xf quickjs-2026-06-04.tar.xz
(cd quickjs-2026-06-04 && make -j1 qjs)
mkdir -p quickjs-master && tar xzf qjs-master.tar.gz -C quickjs-master --strip-components=1
(cd quickjs-master && make -j1 qjs)
tar xzf cmake.tar.gz
mkdir -p quickjs-ng && tar xzf ng.tar.gz -C quickjs-ng --strip-components=1
(cd quickjs-ng && PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" make -j1 JOBS=1)
tar xf node.tar.xz
# Hermes V1 release 260318099.0.7 (b464dba92ad8), interpreter only: the
# release has no x86-64 JIT, and HERMESVM_ALLOW_JIT=0 keeps the option off.
# Needs clang and a C++ toolchain. Release build, Intl off, as the V8 suite
# session of 2026-11-14 built it.
get https://codeload.github.com/facebook/hermes/tar.gz/b464dba92ad87bcfb77fae381a49a2394d9836d9 hermes.tar.gz 57cc6880783cdcd2ca25db49be5332671ca7d358c22ebfe08ff354ff131ed6d6
mkdir -p hermes && tar xzf hermes.tar.gz -C hermes --strip-components=1
(cd hermes && PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" cmake -S . -B build-rel -G "Unix Makefiles" -DCMAKE_BUILD_TYPE=Release -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DHERMES_ENABLE_INTL=OFF -DHERMESVM_ALLOW_JIT=0 &&
  PATH="$PWD/../cmake-3.31.6-linux-x86_64/bin:$PATH" cmake --build build-rel --target hermes hermesc -j 4)
