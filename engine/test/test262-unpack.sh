# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Sourced by test262.sh and its test: unpack_test262 puts test262 with a patch
# applied into a folder, once, and again when the archive or the patch change.

if command -v sha256sum >/dev/null 2>&1; then sum() { sha256sum "$1" | cut -d' ' -f1; }
else sum() { shasum -a 256 "$1" | cut -d' ' -f1; }; fi

# unpack_test262 RUN ARCHIVE ZIPNAME PATCH: RUN/test262 is the archive (which
# holds a folder test262-COMMIT, COMMIT being ZIPNAME without .zip) with PATCH
# applied. RUN/test262.stamp says which archive and which patch it was made
# from, written only after both worked, so a folder left half made, or made
# from the pin before the last change of the commit, the archive or the patch,
# is made again and never run.
unpack_test262() {
	_run=$1 _archive=$2 _zipname=$3 _patch=$4
	_stamp="$_zipname $(sum "$_archive") $(sum "$_patch")"
	if [ -d "$_run/test262" ] && [ -f "$_run/test262.stamp" ] && [ "$(cat "$_run/test262.stamp")" = "$_stamp" ]; then
		return 0
	fi
	_tmp=$_run/test262.unpack
	rm -rf "$_run/test262" "$_run/test262.stamp" "$_tmp"
	# Each step is checked on its own: the caller may run the function where
	# set -e does not apply. The archive is unpacked into a folder of its own
	# and moved into place, so an unzip that was stopped leaves nothing that the
	# next one trips on (it asks before it overwrites, and nothing answers).
	command -v unzip > /dev/null 2>&1 || { echo "test262: unzip not found: install unzip (apt install unzip)" >&2; return 1; }
	mkdir -p "$_tmp" || return 1
	unzip -q "$_archive" -d "$_tmp" < /dev/null || return 1
	mv "$_tmp/test262-${_zipname%.zip}" "$_run/test262" || return 1
	rmdir "$_tmp" || return 1
	patch -s -p1 -d "$_run/test262" < "$_patch" || return 1
	printf '%s\n' "$_stamp" > "$_run/test262.stamp" || return 1
}
