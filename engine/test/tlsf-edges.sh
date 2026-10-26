#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Run the edge cases of TLSF (tlsf-edges.c) on engine/tlsf.c and on the file
# of the project it comes from.
#
#   engine/test/tlsf-edges.sh [-z DIR]
#
# The pinned file is tlsf.c and tlsf.h of github.com/mattconte/tlsf at commit
# deff9ab509341f264addbd3c8ada533678591905, downloaded unless DIR holds them
# (checked against their SHA-256 either way) and converted from CRLF to LF.
# The cases the port fixes must fail on it and every case must pass on
# engine/tlsf.c. CC picks the compiler (default clang) and CFLAGS replaces the
# flags (default: -O1 with AddressSanitizer and UndefinedBehaviorSanitizer,
# which stop the program that does something undefined). Output goes to
# engine/test/out/tlsf-edges/. Exit status 0 means both runs were as expected.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
engine=$(dirname "$here")
cc=${CC:-clang}
cflags=${CFLAGS:--O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all}
dir=
while getopts z: opt; do
	case $opt in
	z) dir=$OPTARG ;;
	*) echo "usage: tlsf-edges.sh [-z DIR]" >&2; exit 2 ;;
	esac
done

commit=deff9ab509341f264addbd3c8ada533678591905
sum_c=2a0f8cfc9cfe6114ccdc6cf22339059440b16f1149b5107bead4ae4c3a0d50e2
sum_h=f7f73c48810ba60203095667c226e5a600a6ea0f69afba48efff6efbaa628d4f
if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum "$1" | cut -d' ' -f1; }
else sum() { shasum -a 256 "$1" | cut -d' ' -f1; }; fi

out=$here/out/tlsf-edges
mkdir -p "$out/orig" "$out/pinned"
for f in tlsf.c tlsf.h; do
	if [ -n "$dir" ]; then cp "$dir/$f" "$out/orig/$f"
	elif [ ! -f "$out/orig/$f" ]; then
		curl -fsSL -o "$out/orig/$f" "https://raw.githubusercontent.com/mattconte/tlsf/$commit/$f"
	fi
done
[ "$(sum "$out/orig/tlsf.c")" = "$sum_c" ] || { echo "tlsf-edges.sh: tlsf.c is not the file at $commit (SHA-256)" >&2; exit 2; }
[ "$(sum "$out/orig/tlsf.h")" = "$sum_h" ] || { echo "tlsf-edges.sh: tlsf.h is not the file at $commit (SHA-256)" >&2; exit 2; }
for f in tlsf.c tlsf.h; do sed 's/\r$//' "$out/orig/$f" > "$out/pinned/$f"; done

# A sanitizer that stops a case exits with 86, which the program reads as a crash.
ASAN_OPTIONS=exitcode=86:detect_leaks=0${ASAN_OPTIONS:+:$ASAN_OPTIONS}
UBSAN_OPTIONS=exitcode=86${UBSAN_OPTIONS:+:$UBSAN_OPTIONS}
export ASAN_OPTIONS UBSAN_OPTIONS

status=0
for which in pinned patched; do
	if [ $which = pinned ]; then inc=$out/pinned; else inc=$engine; fi
	# shellcheck disable=SC2086
	$cc $cflags -I"$inc" "$here/tlsf-edges.c" -o "$out/tlsf-edges-$which"
	echo "== $which ($inc/tlsf.c)"
	"$out/tlsf-edges-$which" $which 2> "$out/$which.err" || status=1
done
exit $status
