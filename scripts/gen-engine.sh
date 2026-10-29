#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# gen-engine.sh [ARCH]: generate the C engine of engine/ as Go assembly into
# internal/cengine (make gen). Run it from anywhere inside the repository on a
# machine with LLVM 22.1.8. ARCH is amd64 (default) or arm64. ARCH pass only
# builds the LLVM pass into .build/shadow.so, which the tests of
# internal/goasm/gen need (scripts/check.sh does it before them).
#
# Environment:
#   LLVM_BIN     bin folder of LLVM 22.1.8 (default /usr/lib/llvm-22/bin)
#   GEN_JOBS     parallel clang jobs (default 2)
#   GEN_SHADOW   log2 of the shadow stack (default 25, the 32 MiB the host uses)
#   GEN_KEEP     copy the generated files to this folder
#   GEN_OUT      write to this folder instead of internal/cengine (make check-gen
#                regenerates into one and compares)
#   GEN_RELEASE  1 (default) is a formal build. 0 allows the survey and test
#                switches of the generator and prints a warning.
set -eu

arch=${1:-amd64}
case $arch in amd64|arm64|pass) ;; *) echo "gen-engine.sh: ARCH $arch: want amd64, arm64 or pass" >&2; exit 2 ;; esac
llvm=${LLVM_BIN:-/usr/lib/llvm-22/bin}
want=22.1.8

cd "$(dirname "$0")/.."
out=${GEN_OUT:-internal/cengine}
got=$("$llvm/clang" --version 2>/dev/null | sed -n 's/.*clang version \([0-9][0-9.]*\).*/\1/p' | head -n 1)
if [ "$got" != "$want" ]; then
	echo "gen-engine.sh: clang in $llvm is '${got:-missing}': the engine is generated with LLVM $want only (set LLVM_BIN)" >&2
	exit 1
fi

export CGO_ENABLED=0
mkdir -p .build

# The pass is rebuilt when one of its sources is newer.
pass=.build/shadow.so
src="internal/goasm/gen/pass/shadow.cpp internal/goasm/gen/pass/poll.cpp internal/goasm/gen/pass/icall.cpp internal/goasm/gen/pass/facts.cpp"
stale=0
[ -f $pass ] || stale=1
for f in $src; do [ "$f" -nt $pass ] && stale=1; done
if [ $stale = 1 ]; then
	# shellcheck disable=SC2046
	"$llvm/clang++" -shared -fPIC -O1 $("$llvm/llvm-config" --cxxflags) $src -o $pass
else
	echo "the pass is up to date"
fi
[ "$arch" = pass ] && exit 0

go build -o .build/engine-gen ./internal/goasm/gen/cmd/engine
rm -rf ".build/w-$arch"

release=-release
if [ "${GEN_RELEASE:-1}" = 0 ]; then
	release=
	echo "warning: GEN_RELEASE=0 is not a formal build" >&2
fi
# shellcheck disable=SC2086
.build/engine-gen -engine engine -llvm "$llvm" -pass $pass -arch "$arch" -mode pn -varargs-block \
	-shadow-size-log2="${GEN_SHADOW:-25}" -pkg cengine -work ".build/w-$arch" -out "$out" \
	-j "${GEN_JOBS:-2}" $release

if [ -n "${GEN_KEEP:-}" ]; then
	rm -rf "$GEN_KEEP"
	mkdir -p "$GEN_KEEP"
	cp "$out"/*_"$arch".s "$out"/engine_*_"$arch".go "$GEN_KEEP"/
	echo "kept a copy in $GEN_KEEP"
fi
