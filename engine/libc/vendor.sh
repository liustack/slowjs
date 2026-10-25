#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Copy the files this library takes from musl, or check the copies.
#
#   engine/libc/vendor.sh MUSL_DIR   copy the files out of a musl checkout at the
#                                    pinned commit, apply patches/, and write MUSL.sha256
#   engine/libc/vendor.sh --verify   check the files here against MUSL.sha256
#
# MUSL.sha256 has one line per file: the SHA-256 of the file in musl, the
# SHA-256 of the file here, the path in musl and the path here. They differ only
# for a file that patches/ changes. The commit is on the first line.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
commit=7ada6dde6f9dc6a2836c3d92c2f762d35fd229e0
manifest=$here/MUSL.sha256

if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum "$1" | cut -d' ' -f1; }
else sum() { shasum -a 256 "$1" | cut -d' ' -f1; }; fi

# musl path -> path here, then the patch for the file when there is one.
files() {
	cat <<LIST
COPYRIGHT COPYRIGHT
src/internal/libm.h internal/libm.h libm.h.patch
src/string/bcmp.c string/bcmp.c
src/string/memchr.c string/memchr.c
src/string/memcmp.c string/memcmp.c
src/string/memcpy.c string/memcpy.c
src/string/memmove.c string/memmove.c
src/string/memrchr.c string/memrchr.c
src/string/memset.c string/memset.c
src/string/strchr.c string/strchr.c
src/string/strchrnul.c string/strchrnul.c
src/string/strcmp.c string/strcmp.c
src/string/strlen.c string/strlen.c
src/string/strrchr.c string/strrchr.c
LIST
	for f in __cos __expo2 __math_divzero __math_invalid __math_oflow __math_uflow \
		__math_xflow __rem_pio2 __rem_pio2_large __sin __tan acos acosh asin asinh \
		atan atan2 atanh cbrt ceil copysign cos cosh exp exp_data expm1 floor fmax \
		fmin fmod hypot log log10 log1p log2 log2_data log_data lrint modf pow \
		pow_data rint round scalbn sin sinh tan tanh trunc; do
		echo "src/math/$f.c math/$f.c"
	done
	for f in exp_data log2_data log_data pow_data; do
		echo "src/math/$f.h math/$f.h"
	done
}

if [ "${1:-}" = --verify ]; then
	[ "$(sed -n 1p "$manifest")" = "# musl $commit" ] || { echo "vendor.sh: $manifest names another commit" >&2; exit 1; }
	status=0
	while read -r orig result mpath path; do
		case $orig in '#'*) continue ;; esac
		[ "$(sum "$here/$path" 2>/dev/null)" = "$result" ] || { echo "vendor.sh: $path differs from MUSL.sha256" >&2; status=1; }
	done < "$manifest"
	n=$(files | wc -l | tr -d ' ')
	[ "$(grep -vc '^#' "$manifest")" = "$n" ] || { echo "vendor.sh: MUSL.sha256 does not list the files this script copies" >&2; status=1; }
	[ $status = 0 ] && echo "vendor.sh: the $n files are the ones in MUSL.sha256"
	exit $status
fi

musl=${1:?usage: vendor.sh MUSL_DIR | --verify}
[ "$(git -C "$musl" rev-parse HEAD)" = "$commit" ] || { echo "vendor.sh: $musl is not at $commit" >&2; exit 1; }
git -C "$musl" diff --quiet HEAD || { echo "vendor.sh: $musl has changes" >&2; exit 1; }

{
	echo "# musl $commit"
	files | while read -r mpath path patch; do
		mkdir -p "$(dirname "$here/$path")"
		cp "$musl/$mpath" "$here/$path"
		orig=$(sum "$musl/$mpath")
		if [ -n "$patch" ]; then
			patch -s "$here/$path" < "$here/patches/$patch"
			rm -f "$here/$path.orig"
		fi
		echo "$orig $(sum "$here/$path") $mpath $path"
	done
} > "$manifest"
echo "vendor.sh: wrote $manifest"
