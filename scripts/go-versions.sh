#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# go-versions.sh [VERSION...]: check the engine with every Go release the
# module lets build: the one of the go line of go.mod and each later patch
# release, up to the minor version internal/goversion/newer.go stops. Or with
# the releases given (such as go1.27.1). The go command fetches each release
# (GOTOOLCHAIN), so it needs the network the first time. One line per release
# at the end. See docs/testing.md.
#
# The checks of each release:
#   go          go build, go vet, and the Go tests of the engine (internal/cengine
#               and the host packages with the testhooks tag, then the root package)
#   goasm       the tests of the generator (internal/goasm), which build sample
#               programs, link them and check the stack check, the call of
#               morestack and which functions are NOSPLIT
#   postlink-X  the post-link check of the whole engine of architecture X (amd64
#               and arm64) against a test binary of internal/cengine linked with
#               this release for each system the module builds for on X: linux,
#               darwin and windows for amd64, linux and darwin for arm64, as go
#               builds them by default (-verify-bin, scripts/gen-engine.sh)
# goasm and postlink need LLVM 22.1.8 (see GO_VERSIONS_LLVM) and write the engine
# into .build only. internal/cengine stays as it is.
#
# Environment:
#   GO_VERSIONS_LOGS  folder of the logs, one file per release (default .build/go-versions)
#   GO_VERSIONS_LIST  1: print the releases and stop
#   GO_VERSIONS_LLVM  1: run goasm and postlink, and stop if LLVM is not there.
#                     0: leave them out. Default: run them when clang is in LLVM_BIN
#   LLVM_BIN          bin folder of LLVM 22.1.8 (default /usr/lib/llvm-22/bin)
set -u

cd "$(dirname "$0")/.." || exit 2
logs=${GO_VERSIONS_LOGS:-.build/go-versions}
mkdir -p "$logs" || exit 2

if [ $# -gt 0 ]; then
	versions=$*
else
	low=$(sed -n 's/^go \(1\.[0-9]*\.[0-9]*\)$/\1/p' go.mod)
	stop=$(sed -n 's/^\/\/go:build go1\.\([0-9]*\) && !quickjs_anygo$/\1/p' internal/goversion/newer.go)
	if [ -z "$low" ] || [ -z "$stop" ]; then
		echo "go-versions.sh: no go line in go.mod or no build line in internal/goversion/newer.go" >&2
		exit 2
	fi
	lowminor=${low#1.}
	lowminor=${lowminor%%.*}
	lowpatch=${low##*.}
	platform=$(go env GOHOSTOS)-$(go env GOHOSTARCH)
	# The toolchains the go command can fetch, as v0.0.1-go1.N.P.GOOS-GOARCH.
	versions=$(go list -m -versions golang.org/toolchain | tr ' ' '\n' |
		sed -n "s/^v0\.0\.1-go\(1\.[0-9]*\.[0-9]*\)\.$platform\$/\1/p" |
		awk -F. -v lm="$lowminor" -v lp="$lowpatch" -v stop="$stop" \
			'($2 > lm || ($2 == lm && $3 >= lp)) && $2 < stop { print "go" $0 }
			END { print "go1." lm "." lp }' |
		sort -u -t. -k2,2n -k3,3n)
	if [ -z "$versions" ]; then
		echo "go-versions.sh: found no Go release from $low up to go1.$stop for $platform" >&2
		exit 2
	fi
fi

if [ "${GO_VERSIONS_LIST:-}" = 1 ]; then
	echo "$versions"
	exit 0
fi

llvm=${LLVM_BIN:-/usr/lib/llvm-22/bin}
use_llvm=${GO_VERSIONS_LLVM:-auto}
case $use_llvm in
auto) if [ -x "$llvm/clang" ]; then use_llvm=1; else use_llvm=0; fi ;;
0 | 1) ;;
*) echo "go-versions.sh: GO_VERSIONS_LLVM: want 0, 1 or unset" >&2; exit 2 ;;
esac
if [ "$use_llvm" = 1 ]; then
	# The pass of the generator does not depend on the Go release.
	if ! LLVM_BIN=$llvm scripts/gen-engine.sh pass >"$logs/pass.log" 2>&1; then
		cat "$logs/pass.log" >&2
		echo "go-versions.sh: cannot build the LLVM pass (GO_VERSIONS_LLVM=0 leaves goasm and postlink out)" >&2
		exit 2
	fi
fi

status=0
summary=
for v in $versions; do
	log=$logs/$v.log
	t0=$(date +%s)
	(
		export GOTOOLCHAIN=$v GOFLAGS= CGO_ENABLED=0 LLVM_BIN=$llvm
		set -e
		step() { # step NAME CMD...
			echo "== $1"
			shift
			"$@"
		}
		postlink() { # postlink ARCH SYSTEM...: the engine of ARCH against a test binary built by this release for each system
			arch=$1
			shift
			bins=
			for os in "$@"; do
				bin=$logs/$v.$os-$arch.test
				GOOS=$os GOARCH=$arch go test -c -tags goasmkeep -o "$bin" ./internal/cengine
				bins=${bins:+$bins,}$bin
			done
			GEN_OUT=$logs/$v.$arch.gen GEN_VERIFY_BIN=$bins scripts/gen-engine.sh "$arch"
			# shellcheck disable=SC2046
			rm -rf "$logs/$v.$arch.gen" $(echo "$bins" | tr , " ")
		}
		go version
		test "$(go env GOVERSION)" = "$v"
		step go-build go build ./...
		step go-vet go vet -asmdecl=false ./...
		step go-test-internal go test -count=1 -tags testhooks ./internal/cengine ./internal/gohost ./internal/hostmem ./internal/idtable
		step go-test-root go test -count=1 -p 1 -timeout 60m .
		if [ "$use_llvm" = 1 ]; then
			step goasm env CHECK_ONLY=goasm CHECK_LOGS="$logs/$v.check" scripts/check.sh fast
			step postlink-amd64 postlink amd64 linux darwin windows
			step postlink-arm64 postlink arm64 linux darwin
		fi
	) >"$log" 2>&1
	rc=$?
	t=$(( $(date +%s) - t0 ))
	if [ $rc -eq 0 ]; then
		summary="$summary
ok    $v  (${t}s)"
	else
		summary="$summary
FAIL  $v  (${t}s, $log)"
		status=1
	fi
done
echo "$summary" | sed 1d
exit $status
