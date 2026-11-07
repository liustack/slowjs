#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test the list of releases of go-versions.sh with a stand-in for go: a list
# that cannot be fetched, or read, stops the script, and a good list gives the
# releases from the go line of go.mod up to the stop of internal/goversion.
#
#   scripts/go-versions-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$here")
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT
mkdir "$d/bin"

low=$(sed -n 's/^go \(1\.[0-9]*\.[0-9]*\)$/\1/p' "$root/go.mod")
stop=$(sed -n 's/^\/\/go:build go1\.\([0-9]*\) && !quickjs_anygo$/\1/p' "$root/internal/goversion/newer.go")
minor=${low#1.}
minor=${minor%%.*}
patch=${low##*.}

# The stand-in answers go env and go list from the files LIST and LISTRC.
cat > "$d/bin/go" <<'EOS'
#!/bin/sh
case $1 in
env) case $2 in GOHOSTOS) echo linux ;; GOHOSTARCH) echo amd64 ;; esac ;;
list) cat "$LIST"; exit "$LISTRC" ;;
*) echo "go-versions-test: unexpected go $*" >&2; exit 99 ;;
esac
EOS
chmod +x "$d/bin/go"

# list: the line go list prints, with the toolchains of the module
{
	printf 'golang.org/toolchain'
	for p in $(seq "$patch" 9); do printf ' v0.0.1-go1.%s.%s.linux-amd64 v0.0.1-go1.%s.%s.darwin-arm64' "$minor" "$p" "$minor" "$p"; done
	printf ' v0.0.1-go1.%s.0.linux-amd64 v0.0.1-go1.%s.1.linux-amd64\n' "$((stop))" "$((stop))"
} > "$d/good"
printf 'golang.org/toolchain\n' > "$d/empty"
{
	printf 'golang.org/toolchain'
	for p in $(seq "$((patch + 1))" 9); do printf ' v0.0.1-go1.%s.%s.linux-amd64' "$minor" "$p"; done
	printf '\n'
} > "$d/nolow"

run() { # run NAME LISTFILE LISTRC EXPECT(pass|fail)
	set +e
	LIST=$2 LISTRC=$3 PATH="$d/bin:$PATH" GO_VERSIONS_LIST=1 "$here/go-versions.sh" > "$d/out" 2> "$d/err"
	rc=$?
	set -e
	if [ "$4" = pass ] && [ "$rc" -ne 0 ]; then echo "go-versions-test: $1: should pass, got $rc"; cat "$d/err"; exit 1; fi
	if [ "$4" = fail ] && [ "$rc" -eq 0 ]; then echo "go-versions-test: $1: should fail, passed:"; cat "$d/out"; exit 1; fi
	echo "go-versions-test: $1: $4, as expected"
}

run "go list fails" "$d/good" 1 fail
run "go list prints nothing and fails" /dev/null 1 fail
run "the list has no toolchain of this system" "$d/empty" 0 fail
run "the list lacks the release of the go line" "$d/nolow" 0 fail
run "a good list" "$d/good" 0 pass
# the releases: the go line up to the patch 9 of its minor, none of the stop
want=$(for p in $(seq "$patch" 9); do echo "go1.$minor.$p"; done)
[ "$(cat "$d/out")" = "$want" ] || { echo "go-versions-test: wrong releases:"; cat "$d/out"; exit 1; }
