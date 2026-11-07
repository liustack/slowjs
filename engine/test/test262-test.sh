#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the unpacking of test262 (test262-unpack.sh): the folder is made again
# when the archive or the patch change, left alone when they do not, and a
# folder left half made is never used.
#
#   engine/test/test262-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
. "$here/test262-unpack.sh"
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT

# archive N: a zip with the folder test262-abc whose file t.js says N, made
# with git archive, which every machine that has the repository has.
archive() {
	rm -rf "$d/src" "$d/arch.zip"
	mkdir -p "$d/src/test262-abc"
	printf 'old %s\n' "$1" > "$d/src/test262-abc/t.js"
	git -C "$d/src" init -q
	git -C "$d/src" add -A
	git -C "$d/src" -c user.name=t -c user.email=t@t commit -q -m t
	git -C "$d/src" archive --format=zip -o "$d/arch.zip" HEAD
}
# patch WORD: a patch that changes "old" in t.js to WORD
patchto() {
	printf -- '--- a/t.js\n+++ b/t.js\n@@ -1 +1 @@\n-old %s\n+%s %s\n' "$1" "$2" "$1" > "$d/p.diff"
}
check() { # check NAME WANT: the file of the folder
	got=$(cat "$d/run/test262/t.js")
	[ "$got" = "$2" ] || { echo "test262-test: $1: t.js is \"$got\", want \"$2\""; exit 1; }
	echo "test262-test: $1: ok"
}

archive 1
patchto 1 new
unpack_test262 "$d/run" "$d/arch.zip" abc.zip "$d/p.diff"
check "first unpack" "new 1"

echo "marker" > "$d/run/test262/marker"
unpack_test262 "$d/run" "$d/arch.zip" abc.zip "$d/p.diff"
[ -f "$d/run/test262/marker" ] || { echo "test262-test: unpacked again with nothing changed"; exit 1; }
echo "test262-test: the same archive and patch are left alone: ok"

archive 2
patchto 2 new
unpack_test262 "$d/run" "$d/arch.zip" abc.zip "$d/p.diff"
check "a new archive" "new 2"

patchto 2 newer
unpack_test262 "$d/run" "$d/arch.zip" abc.zip "$d/p.diff"
check "a new patch" "newer 2"

# a folder with no stamp (a run that stopped, or one made before the stamp)
rm "$d/run/test262.stamp"
echo "stale" > "$d/run/test262/t.js"
unpack_test262 "$d/run" "$d/arch.zip" abc.zip "$d/p.diff"
check "a folder with no stamp" "newer 2"

# an unzip that was stopped left its folder behind, and the next run starts clean
mkdir -p "$d/run3/test262.unpack/test262-abc"
echo stale > "$d/run3/test262.unpack/test262-abc/t.js"
mkdir -p "$d/run3/test262-abc"
echo stale > "$d/run3/test262-abc/t.js"
unpack_test262 "$d/run3" "$d/arch.zip" abc.zip "$d/p.diff" < /dev/null
got=$(cat "$d/run3/test262/t.js")
[ "$got" = "newer 2" ] || { echo "test262-test: after a stopped unzip t.js is \"$got\""; exit 1; }
[ ! -e "$d/run3/test262.unpack" ] || { echo "test262-test: the folder of the unzip is left"; exit 1; }
echo "test262-test: a stopped unzip leaves nothing the next run trips on: ok"

# a patch that does not apply leaves no stamp, so the next run tries again
printf 'bad\n' > "$d/bad.diff"
if (unpack_test262 "$d/run2" "$d/arch.zip" abc.zip "$d/bad.diff") >/dev/null 2>&1; then
	echo "test262-test: a patch that does not apply passed"; exit 1
fi
[ ! -f "$d/run2/test262.stamp" ] || { echo "test262-test: a stamp after a failed patch"; exit 1; }
echo "test262-test: a patch that fails leaves no stamp: ok"
