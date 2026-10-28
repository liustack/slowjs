#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# check-gen.sh (make check-gen): the generated engine checked in under
# internal/cengine is what the generator makes from engine/ now, byte for
# byte, for amd64 and for arm64. It generates into a folder of .build, leaves
# internal/cengine alone, and needs LLVM 22.1.8 (see gen-engine.sh).
#
# A change to the C of engine/ or to the generator goes in with the files this
# check wants: make gen and make gen ARCH=arm64, then commit them with it.
#
# "Checked in" is what git holds (git ls-files), not what the folder holds: a
# file that is ignored or not added is not checked in. The generated files of
# the folder are found by the header the generator writes (Code generated ...
# DO NOT EDIT.), not by their names, so one the generator no longer makes (the
# names it had before the files took an architecture) is a file too many.
#
# Environment: LLVM_BIN, GEN_JOBS as for gen-engine.sh.
set -eu

cd "$(dirname "$0")/.."
dir=internal/cengine
tmp=.build/check-gen
rm -rf "$tmp"
mkdir -p "$tmp"
bad=0
marker='^// Code generated .* DO NOT EDIT\.$'

for arch in amd64 arm64; do
	echo "check-gen: generating $arch"
	if ! GEN_OUT="$tmp/$arch" scripts/gen-engine.sh "$arch" > "$tmp/$arch.log" 2>&1; then
		cat "$tmp/$arch.log" >&2
		echo "check-gen: the generator failed for $arch" >&2
		exit 1
	fi
	# What the generator writes for this architecture, by name.
	(cd "$tmp/$arch" && ls | sort) > "$tmp/$arch.made"
	while read -r f; do
		if [ ! -f "$dir/$f" ]; then
			echo "check-gen: $dir/$f is missing from the folder" >&2
			bad=1
		elif ! cmp -s "$tmp/$arch/$f" "$dir/$f"; then
			echo "check-gen: $dir/$f is not what the generator makes now" >&2
			bad=1
		fi
	done < "$tmp/$arch.made"
done

# The generated files git holds in the folder (its own files, not those of the
# folders below it), by their header in the index, must be the files the
# generator makes for the two architectures and no others.
sort -u "$tmp/amd64.made" "$tmp/arm64.made" > "$tmp/made"
git grep -l --cached -E "$marker" -- "$dir" | sed "s|^$dir/||" | grep -v / | sort > "$tmp/kept" || true
if ! cmp -s "$tmp/made" "$tmp/kept"; then
	echo "check-gen: the files differ in name (< generated now, > in git with a generated header):" >&2
	diff "$tmp/made" "$tmp/kept" >&2 || true
	bad=1
fi
# A generated file git does not hold, ignored or not, is not checked in and
# still takes part in a build of the folder.
git ls-files --others -- "$dir" | sed "s|^$dir/||" | grep -v / | while read -r f; do
	if head -n 3 "$dir/$f" | grep -q -E "$marker"; then
		echo "check-gen: $dir/$f is generated and not in git: add it, or delete it" >&2
		echo bad >> "$tmp/loose"
	fi
done
[ ! -s "$tmp/loose" ] || bad=1

if [ "$bad" = 1 ]; then
	echo "check-gen: run make gen and make gen ARCH=arm64 on a machine with LLVM 22.1.8, and commit the files with the change that needs them" >&2
	exit 1
fi
rm -rf "$tmp"
echo "check-gen: internal/cengine is what the generator makes, for amd64 and arm64"
