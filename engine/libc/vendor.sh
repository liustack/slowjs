#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Copy the files this library takes from musl, or check the copies.
#
#   engine/libc/vendor.sh MUSL_DIR            copy the files out of a musl checkout at the
#                                             pinned commit, apply patches/, and write MUSL.sha256
#   engine/libc/vendor.sh --verify            check the files here against MUSL.sha256
#   engine/libc/vendor.sh --verify MUSL_DIR   the same, and replay the copy from the musl checkout
#
# MUSL.sha256 has the commit on its first line, the SHA-256 of the patches on
# the next, and then one line per file: the SHA-256 of the file in musl, the
# SHA-256 of the file here, the path in musl and the path here. The two differ
# only for a file that patches/ changes.
#
# --verify alone proves that the files here are the ones the list names, that
# no file is missing or added, and that the patch is the recorded one. With a
# musl checkout it proves more: that each file in musl has the recorded hash,
# and that the patch applied to the original gives the file here.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
commit=7ada6dde6f9dc6a2836c3d92c2f762d35fd229e0
manifest=$here/MUSL.sha256

if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum "$1" | cut -d' ' -f1; }
else sum() { shasum -a 256 "$1" | cut -d' ' -f1; }; fi

# musl path, path here, and the patch for the file when there is one.
files() {
	cat <<LIST
COPYRIGHT COPYRIGHT
src/internal/libm.h internal/libm.h libm.h.patch
src/string/bcmp.c string/bcmp.c
src/string/bzero.c string/bzero.c
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

status=0
bad() { echo "vendor.sh: $*" >&2; status=1; }

patch_hash() { # the hashes of the patches, one line each
	for f in "$here"/patches/*; do
		echo "$(sum "$f") patches/$(basename "$f")"
	done
}

if [ "${1:-}" = --verify ]; then
	musl=${2:-}
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	[ "$(sed -n 1p "$manifest")" = "# musl $commit" ] || bad "$manifest names another commit"

	# the patches are the recorded ones
	sed -n 's/^# patch //p' "$manifest" | sort > "$tmp/recorded"
	patch_hash | sort > "$tmp/found"
	cmp -s "$tmp/recorded" "$tmp/found" || bad "patches/ is not what MUSL.sha256 records"

	# the files are the ones the script copies, those the list names, and those here
	files | awk '{print $2}' | sort > "$tmp/script"
	grep -v '^#' "$manifest" | awk '{print $4}' | sort > "$tmp/listed"
	(cd "$here" && find string math -type f; echo internal/libm.h; echo COPYRIGHT) | sort > "$tmp/present"
	cmp -s "$tmp/script" "$tmp/listed" || bad "MUSL.sha256 does not list the files this script copies"
	cmp -s "$tmp/listed" "$tmp/present" || bad "the files here are not the ones MUSL.sha256 lists: $(diff "$tmp/listed" "$tmp/present" | tr '\n' ' ')"

	if [ -n "$musl" ]; then
		[ "$(git -C "$musl" rev-parse HEAD)" = "$commit" ] || bad "$musl is not at $commit"
	fi
	grep -v '^#' "$manifest" | while read -r orig result mpath path; do
		[ "$(sum "$here/$path" 2>/dev/null)" = "$result" ] || bad "$path differs from MUSL.sha256"
		if [ -n "$musl" ]; then
			[ "$(sum "$musl/$mpath")" = "$orig" ] || bad "$mpath in musl is not the recorded file"
			patch=$(files | awk -v p="$path" '$2 == p {print $3}')
			cp "$musl/$mpath" "$tmp/replay"
			[ -z "$patch" ] || patch -s "$tmp/replay" < "$here/patches/$patch"
			[ "$(sum "$tmp/replay")" = "$result" ] || bad "$path is not $mpath with its patch applied"
		fi
		[ $status = 0 ] || exit 1
	done || status=1
	[ $status = 0 ] && echo "vendor.sh: $(grep -vc '^#' "$manifest") files are the ones in MUSL.sha256${musl:+, replayed from $musl}"
	exit $status
fi

musl=${1:?usage: vendor.sh MUSL_DIR | --verify [MUSL_DIR]}
[ "$(git -C "$musl" rev-parse HEAD)" = "$commit" ] || { echo "vendor.sh: $musl is not at $commit" >&2; exit 1; }
git -C "$musl" diff --quiet HEAD || { echo "vendor.sh: $musl has changes" >&2; exit 1; }

{
	echo "# musl $commit"
	patch_hash | sed 's/^/# patch /'
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
