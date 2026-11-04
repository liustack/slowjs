#!/bin/sh
# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# go-versions.sh [VERSION...]: run the Go tests of the engine with the Go
# releases the module lets build: the one of the go line of go.mod, and the
# newest patch release of each minor version from it up to the one
# internal/goversion/newer.go stops. A patch release does not change what the
# assembly leans on, so one for each minor version is enough. Or with the
# releases given (such as go1.27.1). The go command fetches each release
# (GOTOOLCHAIN), so it needs the network the first time. One line per release
# at the end. See docs/testing.md.
#
# Environment:
#   GO_VERSIONS_LOGS  folder of the logs, one file per release (default .build/go-versions)
#   GO_VERSIONS_LIST  1: print the releases and stop
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
			'($2 > lm || ($2 == lm && $3 >= lp)) && $2 < stop {
			if (!($2 in top) || $3 > top[$2]) top[$2] = $3
		}
		END {
			print "go1." lm "." lp
			for (m in top) if (m != lm || top[m] != lp) print "go1." m "." top[m]
		}' |
		sort -t. -k2,2n -k3,3n)
	if [ -z "$versions" ]; then
		echo "go-versions.sh: found no Go release from $low up to go1.$stop for $platform" >&2
		exit 2
	fi
fi

if [ "${GO_VERSIONS_LIST:-}" = 1 ]; then
	echo "$versions"
	exit 0
fi

status=0
summary=
for v in $versions; do
	log=$logs/$v.log
	(
		export GOTOOLCHAIN=$v GOFLAGS=
		set -e
		go version
		test "$(go env GOVERSION)" = "$v"
		go build ./...
		go vet -asmdecl=false ./...
		go test -count=1 -tags testhooks ./internal/cengine ./internal/gohost ./internal/hostmem ./internal/idtable
		go test -count=1 -p 1 -timeout 60m .
	) >"$log" 2>&1
	if [ $? -eq 0 ]; then
		summary="$summary
ok    $v"
	else
		summary="$summary
FAIL  $v  ($log)"
		status=1
	fi
done
echo "$summary" | sed 1d
exit $status
