# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# What a build of the native test host was made from, so that no test runs a
# build of other sources than the tree has. Sourced by build.sh,
# regvm/build.sh and the scripts that run what they build.
#
# build.sh removes OUTDIR/engine.stamp first and writes it at its end, with
# the sources as they were when it started:
#
#   src HASH     every .c and .h under engine/ but engine/test/out and
#                engine/test/regvm, with build.sh and flags.sh: what the
#                engine objects and the programs of engine/test come from
#   mode MODE    the mode of build.sh
#   defs DEFS    EXTRA_DEFS
#   cc CC        the compiler and the first line of its --version
#
# regvm/build.sh builds the engine again when engine.stamp is not that of the
# tree, the mode, EXTRA_DEFS and CC of now, and writes OUTDIR/regvm.stamp:
# the lines of engine.stamp and "tools HASH" of the .c and .h of
# engine/test/regvm with its build.sh. stamp_check refuses a folder whose
# regvm.stamp is missing or names other sources than the tree has now, and
# rvdiff, which has the hashes and the path of its tree built in, refuses to
# run when that tree has other sources now (see rvdiff.c).

# the first 16 hex digits of the SHA-256 of the files named on stdin, names
# and contents
stamp_hash_files() {
	if command -v sha256sum > /dev/null 2>&1; then
		xargs sha256sum | sha256sum | cut -c1-16
	else
		xargs shasum -a 256 | shasum -a 256 | cut -c1-16
	fi
}

# stamp_src ENGINE: the hash of the sources of the engine objects
stamp_src() {
	(
		cd "$1" || exit 1
		LC_ALL=C
		export LC_ALL
		{
			find . -path ./test/out -prune -o -path ./test/regvm -prune \
				-o -type f \( -name '*.c' -o -name '*.h' \) -print
			echo ./test/build.sh
			echo ./test/flags.sh
		} | sort | stamp_hash_files
	)
}

# stamp_tools ENGINE: the hash of the tools of engine/test/regvm
stamp_tools() {
	(
		cd "$1/test/regvm" || exit 1
		LC_ALL=C
		export LC_ALL
		{
			find . -maxdepth 1 -type f \( -name '*.c' -o -name '*.h' \) -print
			echo ./build.sh
		} | sort | stamp_hash_files
	)
}

# stamp_engine_lines ENGINE MODE CC: the lines of engine.stamp for a build of
# the tree now, with EXTRA_DEFS
stamp_engine_lines() {
	echo "src $(stamp_src "$1")"
	echo "mode $2"
	echo "defs ${EXTRA_DEFS:-}"
	echo "cc $3 $("$3" --version 2>/dev/null | head -n 1)"
}

# stamp_engine_fresh ENGINE OUTDIR MODE CC: whether OUTDIR/engine.stamp is
# that of a build of the tree now, with EXTRA_DEFS
stamp_engine_fresh() {
	[ -f "$2/engine.stamp" ] && [ "$(cat "$2/engine.stamp")" = "$(stamp_engine_lines "$1" "$3" "$4")" ]
}

# stamp_check ENGINE BINDIR: fails, saying what to run, when the tools of
# BINDIR were not built from the sources of the tree now. It exports
# RV_TREE_STAMP, the hashes of the tree, which rvdiff then takes in place of
# hashing the tree again at each run.
stamp_check() {
	if [ ! -f "$2/regvm.stamp" ]; then
		echo "$2 has no regvm.stamp, so what it was built from is not known: build it with engine/test/regvm/build.sh MODE $2" >&2
		return 1
	fi
	stamp_s=$(stamp_src "$1")
	stamp_t=$(stamp_tools "$1")
	RV_TREE_STAMP="src $stamp_s tools $stamp_t"
	export RV_TREE_STAMP
	if ! grep -qx "src $stamp_s" "$2/regvm.stamp" || ! grep -qx "tools $stamp_t" "$2/regvm.stamp"; then
		echo "$2 was built from other sources than the tree has now: build it again with engine/test/regvm/build.sh $(sed -n 's/^mode //p' "$2/regvm.stamp") $2" >&2
		return 1
	fi
}
