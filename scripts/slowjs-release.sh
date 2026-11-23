#!/bin/sh
# Copyright (c) 2026 The liustack/slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.
#
# slowjs-release.sh <version> <dir>: builds the slowjs command (cmd/slowjs)
# for the six targets the engine runs on, with <version> stamped in, packs
# each binary with the license texts NOTICE.md says a binary must carry, and
# writes <dir>/checksums.txt. The release job (.github/workflows/release.yml)
# runs it, see docs/internals/releasing.md. Each archive is named after its target as
# Go names it, with no version, and holds one folder of the same name:
#
#   slowjs-linux-amd64.tar.gz
#   slowjs-linux-arm64.tar.gz
#   slowjs-darwin-amd64.tar.gz
#   slowjs-darwin-arm64.tar.gz
#   slowjs-windows-amd64.zip
#   slowjs-windows-arm64.zip
#
# The binaries are static: no cgo, no C library, so the linux ones run on
# glibc and musl systems alike, and the windows ones need no runtime.
set -eu
if [ $# -ne 2 ] || [ -z "$1" ]; then
	echo "usage: $0 <version> <dir>" >&2
	exit 2
fi
version=$1
cd "$(dirname "$0")/.."
mkdir -p "$2"
out=$(cd "$2" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# The license texts, at their paths in this repository, which NOTICE.md
# names: those of the engine, then those of intl and temporal, of url, and
# of fmt.
notices="NOTICE.md LICENSE LICENSE-QUICKJS LICENSE-TLSF engine/libc/COPYRIGHT
	intl/LICENSE-DATA internal/gointl/LICENSE internal/gointl/qjsdata/NOTICE-ICU internal/gointl/qjsdata/NOTICE-ICU4X
	internal/gointl/qjsdata/NOTICE-TEMPORAL internal/gointl/qjsdata/NOTICE-V8
	internal/whatwgurl/LICENSE-WHATWG-URL internal/whatwgurl/LICENSE-GO fmt/LICENSE-GO"
# What else a Go binary links: the runtime and standard library of Go, and
# golang.org/x/sys, the only module the command links besides this one.
goroot=$(go env GOROOT)
xsys=$(go mod download -json golang.org/x/sys | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')
xsysver=$(go list -m -f '{{.Version}}' golang.org/x/sys)
if [ ! -f "$goroot/LICENSE" ] || [ ! -f "$xsys/LICENSE" ]; then
	echo "$0: the LICENSE of Go ($goroot) or of golang.org/x/sys ($xsys) is missing" >&2
	exit 1
fi

for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	goos=${t%/*}
	goarch=${t#*/}
	name=slowjs-$goos-$goarch
	exe=
	[ "$goos" = windows ] && exe=.exe
	dir=$stage/$name
	mkdir -p "$dir"
	echo "$goos/$goarch: $name"
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$dir/slowjs$exe" ./cmd/slowjs

	# A module linked that has no license text here would ship without
	# one: stop instead.
	deps=$(go version -m "$dir/slowjs$exe" | awk '$1 == "dep" { print $2 }')
	if [ "$deps" != golang.org/x/sys ]; then
		echo "$0: the binary links modules this script ships no license for:" $deps >&2
		exit 1
	fi

	for f in $notices; do
		mkdir -p "$dir/$(dirname "$f")"
		cp "$f" "$dir/$f"
	done
	mkdir -p "$dir/third_party/go" "$dir/third_party/golang.org/x/sys@$xsysver"
	cp "$goroot/LICENSE" "$dir/third_party/go/LICENSE"
	cp "$xsys/LICENSE" "$dir/third_party/golang.org/x/sys@$xsysver/LICENSE"
	find "$dir" -type f ! -name "slowjs$exe" -exec chmod 0644 {} +

	rm -f "$out/$name.tar.gz" "$out/$name.zip"
	if [ "$goos" = windows ]; then
		(cd "$stage" && zip -q -r -X "$out/$name.zip" "$name")
	else
		tar -czf "$out/$name.tar.gz" -C "$stage" "$name"
	fi
done

cd "$out"
if command -v sha256sum >/dev/null 2>&1; then
	sha256sum slowjs-*.tar.gz slowjs-*.zip >checksums.txt
else
	shasum -a 256 slowjs-*.tar.gz slowjs-*.zip >checksums.txt
fi
cat checksums.txt
