#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# Test docs-list.sh --check on a copy of the script in a folder of its own:
# a user doc whose Chinese version is translated from it as it is now passes,
# and it fails when the English changes without the Chinese, when the Chinese
# version or its translatedFrom is missing, when a *.zh-CN.md is next to a doc
# that is not a user doc or next to none, when the audience is not user, when
# a doc has no summary and when a README is missing. The hash is taken with
# sha256sum or with shasum, whichever the PATH has, and --check stops when it
# has neither.
#
#   scripts/docs-list-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT
bash=$(command -v bash)
fails=0

# sha FILE: the first 12 hex digits of the SHA-256 of FILE.
if command -v sha256sum > /dev/null 2>&1; then
	sha() { sha256sum < "$1" | cut -c1-12; }
else
	sha() { shasum -a 256 < "$1" | cut -c1-12; }
fi

# setup: a repository with a doc for developers, a user doc and its Chinese
# version translated from it.
setup() {
	rm -rf "$d/r"
	mkdir -p "$d/r/scripts" "$d/r/docs"
	cp "$here/docs-list.sh" "$d/r/scripts/"
	printf '# R\n' > "$d/r/README.md"
	printf '# R\n' > "$d/r/README.zh-CN.md"
	printf -- "---\nsummary: 'For developers'\nread_when:\n  - Always\n---\n\n# Dev\n" > "$d/r/docs/dev.md"
	printf -- "---\nsummary: 'For users'\naudience: user\nread_when:\n  - Using it\n---\n\n# Use\n\nRun it.\n" > "$d/r/docs/use.md"
	zh "$(sha "$d/r/docs/use.md")"
}

# zh HASH: write docs/use.zh-CN.md with translatedFrom: HASH, none when empty.
zh() {
	{
		printf -- "---\nsummary: '给用户'\nread_when:\n  - 用它的时候\n"
		[ -z "$1" ] || printf 'translatedFrom: %s\n' "$1"
		printf -- "---\n\n# 用法\n\n运行它。\n"
	} > "$d/r/docs/use.zh-CN.md"
}

# expect CODE TEXT NAME: --check exits with CODE, and its warnings hold TEXT.
expect() {
	rc=0
	PATH=${P:-$PATH} "$bash" "$d/r/scripts/docs-list.sh" --check > "$d/out" 2> "$d/err" || rc=$?
	if [ "$rc" != "$1" ]; then
		echo "FAIL $3: exit $rc, want $1"; sed 's/^/  /' "$d/err"; fails=$((fails + 1)); return
	fi
	if [ -n "$2" ] && ! grep -qF -- "$2" "$d/err"; then
		echo "FAIL $3: no \"$2\" in the warnings"; sed 's/^/  /' "$d/err"; fails=$((fails + 1)); return
	fi
	echo "ok   $3"
}

setup
expect 0 "" "a user doc and its Chinese version pass"
if grep -q zh-CN "$d/out"; then
	echo "FAIL the index lists the Chinese version"; fails=$((fails + 1))
else
	echo "ok   the index lists the English docs only"
fi

setup
want=$(sha "$d/r/docs/use.md")
printf 'Run it twice.\n' >> "$d/r/docs/use.md"
expect 1 "translatedFrom: $want, now $(sha "$d/r/docs/use.md")" "the English changes without the Chinese"

setup
rm "$d/r/docs/use.zh-CN.md"
expect 1 "docs/use.md is a user doc and has no Chinese version docs/use.zh-CN.md" "a user doc has no Chinese version"

setup
zh ""
expect 1 "docs/use.zh-CN.md has no translatedFrom: $(sha "$d/r/docs/use.md")" "the Chinese has no translatedFrom"

setup
cp "$d/r/docs/use.zh-CN.md" "$d/r/docs/dev.zh-CN.md"
expect 1 "docs/dev.zh-CN.md is next to no user doc" "a Chinese version of a doc for developers"

setup
cp "$d/r/docs/use.zh-CN.md" "$d/r/docs/gone.zh-CN.md"
expect 1 "docs/gone.zh-CN.md is next to no user doc" "a Chinese version of no doc"

setup
sed 's/^audience: user$/audience: users/' "$d/r/docs/use.md" > "$d/x" && mv "$d/x" "$d/r/docs/use.md"
expect 1 "docs/use.md has audience: users, and the only audience is user" "an audience other than user"

setup
sed '/^summary:/d' "$d/r/docs/use.zh-CN.md" > "$d/x" && mv "$d/x" "$d/r/docs/use.zh-CN.md"
expect 1 "docs/use.zh-CN.md has no front matter summary" "the Chinese has no summary"

setup
printf '# No front matter\n' > "$d/r/docs/bare.md"
expect 1 "docs/bare.md has no front matter summary" "a doc has no summary"

setup
rm "$d/r/README.zh-CN.md"
expect 1 "README.zh-CN.md is missing" "a README is missing"

# The PATH of each case holds the tools the script runs, and sha256sum,
# shasum or neither. A stand-in passes the call to the tool the machine has.
mkdir "$d/bin-sha256sum" "$d/bin-shasum" "$d/bin-none"
for b in "$d/bin-sha256sum" "$d/bin-shasum" "$d/bin-none"; do
	for t in dirname find sort awk cut; do ln -s "$(command -v $t)" "$b/$t"; done
done
real256=$(command -v sha256sum || true)
realsha=$(command -v shasum || true)
if [ -n "$real256" ]; then
	ln -s "$real256" "$d/bin-sha256sum/sha256sum"
	printf '#!/bin/sh\n[ "$1 $2" = "-a 256" ] || exit 9\nshift 2\nexec %s "$@"\n' "$real256" > "$d/bin-shasum/shasum"
	chmod +x "$d/bin-shasum/shasum"
else
	ln -s "$realsha" "$d/bin-shasum/shasum"
	printf '#!/bin/sh\nexec %s -a 256 "$@"\n' "$realsha" > "$d/bin-sha256sum/sha256sum"
	chmod +x "$d/bin-sha256sum/sha256sum"
fi

setup
for P in "$d/bin-sha256sum" "$d/bin-shasum"; do
	expect 0 "" "the hash taken with ${P##*-}"
done
P=$d/bin-none
expect 1 "--check needs sha256sum or shasum" "neither sha256sum nor shasum"
unset P

[ "$fails" = 0 ] || { echo "$fails failed"; exit 1; }
echo "all passed"
