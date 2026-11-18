#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# Test go-versions.sh with a stand-in for go: a list of releases that cannot
# be fetched, or read, stops the script, a good list gives the releases from
# the go line of go.mod up to the stop of internal/goversion, and a release
# whose checks fail has the end of its log printed.
#
#   scripts/go-versions-test.sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(dirname "$here")
d=$(mktemp -d)
trap 'rm -rf "$d"' EXIT
mkdir "$d/bin"

low=$(sed -n 's/^go \(1\.[0-9]*\.[0-9]*\)$/\1/p' "$root/go.mod")
stop=$(sed -n 's/^\/\/go:build go1\.\([0-9]*\) && !slowjs_anygo$/\1/p' "$root/internal/goversion/newer.go")
minor=${low#1.}
minor=${minor%%.*}
patch=${low##*.}

# The stand-in answers go env and go list from the files LIST and LISTRC. Its
# go version and go env GOVERSION give the release GOTOOLCHAIN names, go build
# and go vet pass, and go test fails when TESTFAIL is set.
cat > "$d/bin/go" <<'EOS'
#!/bin/sh
case $1 in
env) case $2 in GOHOSTOS) echo linux ;; GOHOSTARCH) echo amd64 ;; GOVERSION) echo "$GOTOOLCHAIN" ;; esac ;;
list) cat "$LIST"; exit "$LISTRC" ;;
version)
	# NOFETCH: a release whose toolchain cannot be fetched. ONCE: a file,
	# present until the first fetch, which fails.
	if [ "$GOTOOLCHAIN" = "${NOFETCH:-}" ] || { [ -n "${ONCE:-}" ] && [ -f "$ONCE" ] && rm "$ONCE"; }; then
		echo "go: downloading $GOTOOLCHAIN (linux/amd64)"
		echo "go: download $GOTOOLCHAIN for linux/amd64: dial tcp: lookup proxy.golang.org: no such host" >&2
		exit 1
	fi
	echo "go version $GOTOOLCHAIN linux/amd64" ;;
build | vet) echo "$GOTOOLCHAIN $1" >> "$CALLS" ;;
test)
	[ -z "${TESTFAIL:-}" ] && exit 0
	echo "--- FAIL: TestStandIn"
	echo "    the line that says why"
	exit 1 ;;
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

# checks NAME EXPECT(pass|fail) VERSION...: the checks of the releases given,
# without LLVM. The output goes to $d/out, together with the errors.
checks() {
	name=$1 expect=$2
	shift 2
	set +e
	: > "$d/calls"
	PATH="$d/bin:$PATH" CALLS="$d/calls" GO_VERSIONS_LLVM=0 GO_VERSIONS_LOGS="$d/logs" GO_VERSIONS_FETCH_PAUSE=0 \
		"$here/go-versions.sh" "$@" > "$d/out" 2>&1
	rc=$?
	set -e
	if [ "$expect" = pass ] && [ "$rc" -ne 0 ]; then echo "go-versions-test: $name: should pass, got $rc"; cat "$d/out"; exit 1; fi
	if [ "$expect" = fail ] && [ "$rc" -ne 1 ]; then echo "go-versions-test: $name: should fail with 1, got $rc"; cat "$d/out"; exit 1; fi
	if [ "$expect" = stop ] && [ "$rc" -ne 2 ]; then echo "go-versions-test: $name: should stop with 2, got $rc"; cat "$d/out"; exit 1; fi
	echo "go-versions-test: $name: $expect, as expected"
}

checks "the checks pass" pass go1.99.1
grep -q 'the line that says why' "$d/out" && { echo "go-versions-test: a release that passed had its log printed:"; cat "$d/out"; exit 1; }
# An assignment before a function call stays set in some shells: unset it after.
export TESTFAIL=1
checks "a test fails" fail go1.99.1
unset TESTFAIL
# the reason is in what the script prints, not only in a log on the machine
grep -q 'the line that says why' "$d/out" || { echo "go-versions-test: the end of the failed log is not printed:"; cat "$d/out"; exit 1; }

# Every toolchain is fetched before the first check: one that cannot be
# fetched stops the script before it checks anything, and says why.
export NOFETCH=go1.99.2
checks "a toolchain cannot be fetched" stop go1.99.1 go1.99.2
unset NOFETCH
[ -s "$d/calls" ] && { echo "go-versions-test: checks ran before every toolchain was fetched:"; cat "$d/calls"; exit 1; }
grep -q 'no such host' "$d/out" || { echo "go-versions-test: the reason of the failed fetch is not printed:"; cat "$d/out"; exit 1; }
# a fetch that fails once is tried again
touch "$d/once"
export ONCE="$d/once"
checks "a fetch fails once" pass go1.99.1
unset ONCE
