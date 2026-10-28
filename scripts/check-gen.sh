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
# file that is ignored or not added is not checked in.
#
# Environment: LLVM_BIN, GEN_JOBS as for gen-engine.sh.
set -eu

cd "$(dirname "$0")/.."
dir=internal/cengine
tmp=.build/check-gen
rm -rf "$tmp"
mkdir -p "$tmp"
bad=0

for arch in amd64 arm64; do
	echo "check-gen: generating $arch"
	if ! GEN_OUT="$tmp/$arch" scripts/gen-engine.sh "$arch" > "$tmp/$arch.log" 2>&1; then
		cat "$tmp/$arch.log" >&2
		echo "check-gen: the generator failed for $arch" >&2
		exit 1
	fi
	# What the generator writes for this architecture, by name: the files
	# of that name in the checked-in folder must be the same set.
	(cd "$tmp/$arch" && ls | sort) > "$tmp/$arch.made"
	git ls-files -- "$dir" | sed "s|^$dir/||" | grep -E "^[^/]*_$arch\.s$|^engine_(decl|keep|negative)_$arch\.go$" | sort > "$tmp/$arch.kept" || true
	if ! cmp -s "$tmp/$arch.made" "$tmp/$arch.kept"; then
		echo "check-gen: $arch: the files differ in name (< generated now, > checked in):" >&2
		diff "$tmp/$arch.made" "$tmp/$arch.kept" >&2 || true
		bad=1
	fi
	while read -r f; do
		if [ ! -f "$dir/$f" ]; then
			echo "check-gen: $dir/$f is missing from the folder" >&2
			bad=1
		elif ! cmp -s "$tmp/$arch/$f" "$dir/$f"; then
			echo "check-gen: $dir/$f is not what the generator makes now" >&2
			bad=1
		fi
	done < "$tmp/$arch.made"
	# A generated file nobody added to git, or git ignores, is as good as not
	# checked in.
	loose=$(cd "$dir" && git ls-files --others -- "*_$arch.s" "engine_*_$arch.go" | { grep -v / || true; })
	if [ -n "$loose" ]; then
		echo "check-gen: $arch: generated files not in git: $loose" >&2
		bad=1
	fi
done

if [ "$bad" = 1 ]; then
	echo "check-gen: run make gen and make gen ARCH=arm64 on a machine with LLVM 22.1.8, and commit the files with the change that needs them" >&2
	exit 1
fi
rm -rf "$tmp"
echo "check-gen: internal/cengine is what the generator makes, for amd64 and arm64"
