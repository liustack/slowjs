# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# cover.awk: the check train.sh makes of loops.js, over three profiles in the
# text form of llvm-profdata, in this order: the pages, the micro benchmarks of
# compare/testdata (run for the check only, never trained on), and loops.js.
#
#   awk -v cold=C -v hot=H -v floor=F -v need="NAME..." -f cover.awk pages.txt micro.txt loops.txt
#
# A path here is one counter of the profile. The paths the loops are for are
# those the pages leave cold (a count below C, the cold count of the pages'
# profile) and the micro benchmarks run at least H times. It prints how many
# of them loops.js runs, and fails when that is below F percent. Each NAME
# (a function, without the file of a static one) must run in loops.js: it
# names the path of one part of the file, and a part that no longer reaches
# its function fails the check by name.

FNR == 1 { file++ }
/^$/ { next }
/^#/ { vals = ($0 ~ /^# Counter Values:/); if (vals) i = 0; next }
/^[0-9]+$/ {
	if (vals) {
		i++
		c[file, fn, i] = $1
		if (file == 1) n[fn] = i
	}
	next
}
{ fn = $0; vals = 0 }

END {
	for (f in n)
		for (k = 1; k <= n[f]; k++)
			if (c[1, f, k] + 0 < cold && c[2, f, k] + 0 >= hot) {
				total++
				if (c[3, f, k] + 0 > 0) got++
				else miss[f]++
			}
	bad = 0
	m = split(need, names, " ")
	for (j = 1; j <= m; j++) {
		found = 0
		for (f in n) {
			b = f
			sub(/^.*:/, "", b)
			if (b == names[j]) { found = 1; if (c[3, f, 1] + 0 == 0) { printf "train.sh: loops.js no longer runs %s\n", names[j]; bad = 1 } }
		}
		if (!found) { printf "train.sh: no function %s in the profile of the pages\n", names[j]; bad = 1 }
	}
	if (total == 0) { print "train.sh: the micro benchmarks run no path the pages leave cold: the check is broken"; exit 1 }
	pct = 100 * got / total
	printf "engine/pgo/loops.js: runs %d of the %d paths the pages leave cold and the micro benchmarks of compare/testdata run (%.1f%%, at least %d%%)\n", got, total, pct, floor
	if (pct < floor) {
		print "train.sh: loops.js runs too few of them. The functions with the most paths it misses:"
		for (f in miss) print miss[f], f | "sort -rn | head -10"
		close("sort -rn | head -10")
		bad = 1
	}
	exit bad
}
