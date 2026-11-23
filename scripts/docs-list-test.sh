#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# Test docs-list.sh --check on a copy of the script in a folder of its own:
# a user doc in docs/use/ whose Chinese version is translated from it as it is
# now passes, and it fails when the English changes without the Chinese, when
# the Chinese version or its translatedFrom is missing, when the audience or
# the order of the Chinese differs, when a *.zh-CN.md is next to a doc that is
# not a user doc or next to none, when a doc has no audience or an audience
# that is not user or dev, when a doc has no order or one that is not a whole
# number, when a user doc is outside docs/use/ or a dev doc is inside it, when
# a doc has no summary and when a README is missing. It also fails
# when docs/use/index.md, docs/use/index.zh-CN.md or
# docs/internals/index.md is missing. The hash is taken with sha256sum or with
# shasum, whichever the PATH has, and --check stops when it has neither.
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

U=docs/use/guide
# setup: a repository with a doc for developers, a user doc and its Chinese
# version translated from it.
setup() {
	rm -rf "$d/r"
	mkdir -p "$d/r/scripts" "$d/r/docs/use" "$d/r/docs/internals"
	cp "$here/docs-list.sh" "$d/r/scripts/"
	printf '# R\n' > "$d/r/README.md"
	printf '# R\n' > "$d/r/README.zh-CN.md"
	printf -- "---\nsummary: 'For developers'\naudience: dev\norder: 2\nread_when:\n  - Always\n---\n\n# Dev\n" > "$d/r/docs/internals/dev.md"
	printf -- "---\nsummary: 'For users'\naudience: user\norder: 2\nread_when:\n  - Using it\n---\n\n# Use\n\nRun it.\n" > "$d/r/$U.md"
	zh "$(sha "$d/r/$U.md")"
	indexes
}

# zh HASH: write docs/use/guide.zh-CN.md with translatedFrom: HASH, none when
# empty.
zh() {
	{
		printf -- "---\nsummary: '给用户'\naudience: user\norder: 2\nread_when:\n  - 用它的时候\n"
		[ -z "$1" ] || printf 'translatedFrom: %s\n' "$1"
		printf -- "---\n\n# 用法\n\n运行它。\n"
	} > "$d/r/$U.zh-CN.md"
}

# indexes: the three index files.
indexes() {
	printf -- "---\ntitle: Use\naudience: user\norder: 1\n---\n\n# Use\n" > "$d/r/docs/use/index.md"
	printf -- "---\ntitle: 使用\naudience: user\norder: 1\ntranslatedFrom: %s\n---\n\n# 使用\n" "$(sha "$d/r/docs/use/index.md")" > "$d/r/docs/use/index.zh-CN.md"
	printf -- "---\ntitle: Internals\naudience: dev\norder: 2\n---\n\n# Internals\n" > "$d/r/docs/internals/index.md"
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

# edit FILE SED: change a file of the repository with a sed script.
edit() {
	sed "$2" "$d/r/$1" > "$d/x" && mv "$d/x" "$d/r/$1"
}

setup
expect 0 "" "a user doc and its Chinese version pass"
if grep -q zh-CN "$d/out"; then
	echo "FAIL the index lists the Chinese version"; fails=$((fails + 1))
else
	echo "ok   the index lists the English docs only"
fi

setup
want=$(sha "$d/r/$U.md")
printf 'Run it twice.\n' >> "$d/r/$U.md"
expect 1 "translatedFrom: $want, now $(sha "$d/r/$U.md")" "the English changes without the Chinese"

setup
rm "$d/r/$U.zh-CN.md"
expect 1 "$U.md is a user doc and has no Chinese version $U.zh-CN.md" "a user doc has no Chinese version"

setup
zh ""
expect 1 "$U.zh-CN.md has no translatedFrom: $(sha "$d/r/$U.md")" "the Chinese has no translatedFrom"

setup
edit $U.zh-CN.md 's/^order: 2$/order: 3/'
expect 1 "$U.zh-CN.md has order: 3, and $U.md has order: 2" "the Chinese has another order"

setup
edit $U.zh-CN.md '/^audience:/d'
expect 1 "$U.zh-CN.md has audience: , and $U.md has audience: user" "the Chinese has no audience"

setup
cp "$d/r/$U.zh-CN.md" "$d/r/docs/internals/dev.zh-CN.md"
expect 1 "docs/internals/dev.zh-CN.md is next to no user doc" "a Chinese version of a doc for developers"

setup
cp "$d/r/$U.zh-CN.md" "$d/r/docs/use/gone.zh-CN.md"
expect 1 "docs/use/gone.zh-CN.md is next to no user doc" "a Chinese version of no doc"

setup
edit $U.md 's/^audience: user$/audience: users/'
expect 1 "$U.md has audience: users, and the audiences are user and dev" "an audience other than user or dev"

setup
edit docs/internals/dev.md '/^audience:/d'
expect 1 "docs/internals/dev.md has no audience: it must be user or dev" "a doc has no audience"

setup
edit docs/internals/dev.md '/^order:/d'
expect 1 "docs/internals/dev.md has no order: it must be a whole number" "a doc has no order"

setup
edit docs/internals/dev.md 's/^order: 2$/order: two/'
expect 1 "docs/internals/dev.md has order: two, and an order is a whole number" "an order that is not a number"

setup
mv "$d/r/$U.md" "$d/r/docs/guide.md"
mv "$d/r/$U.zh-CN.md" "$d/r/docs/guide.zh-CN.md"
expect 1 "docs/guide.md is a user doc and is not in docs/use/" "a user doc outside docs/use/"

setup
mv "$d/r/docs/internals/dev.md" "$d/r/docs/use/dev.md"
expect 1 "docs/use/dev.md is a dev doc and is in docs/use/" "a dev doc inside docs/use/"

setup
edit $U.zh-CN.md '/^summary:/d'
expect 1 "$U.zh-CN.md has no front matter summary" "the Chinese has no summary"

setup
printf '# No front matter\n' > "$d/r/docs/internals/bare.md"
expect 1 "docs/internals/bare.md has no front matter summary" "a doc has no summary"

setup
rm "$d/r/README.zh-CN.md"
expect 1 "README.zh-CN.md is missing" "a README is missing"

# The index files: listed nowhere, and each one needed.
setup
expect 0 "" "the index files pass, listed nowhere"
if grep -q index "$d/out"; then
	echo "FAIL the index lists an index file"; fails=$((fails + 1))
else
	echo "ok   the index files are not listed"
fi

setup
rm "$d/r/docs/use/index.md" "$d/r/docs/use/index.zh-CN.md" "$d/r/docs/internals/index.md"
expect 1 "docs/use/index.md is missing" "no index files"
grep -qF "docs/use/index.zh-CN.md is missing" "$d/err" && grep -qF "docs/internals/index.md is missing" "$d/err" \
	&& echo "ok   each missing index is named" \
	|| { echo "FAIL not every missing index is named"; fails=$((fails + 1)); }

setup
rm "$d/r/docs/internals/index.md"
expect 1 "docs/internals/index.md is missing" "no docs/internals/index.md"

setup
edit docs/use/index.md '/^title:/d'
expect 1 "docs/use/index.md has no title" "an index has no title"

setup
edit docs/use/index.md 's/^audience: user$/audience: dev/'
expect 1 "docs/use/index.md is a dev doc and is in docs/use/" "an index with the audience of the other folder"

# The PATH of each case holds the tools the script runs, and sha256sum,
# shasum or neither. A stand-in passes the call to the tool the machine has.
mkdir "$d/bin-sha256sum" "$d/bin-shasum" "$d/bin-none"
for b in "$d/bin-sha256sum" "$d/bin-shasum" "$d/bin-none"; do
	for t in dirname find sort awk cut grep; do ln -s "$(command -v $t)" "$b/$t"; done
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
