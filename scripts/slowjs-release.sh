#!/bin/sh
# Copyright (c) 2026 The liustack/quickjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
#
# slowjs-release.sh <version> <dir>: builds the slowjs command (cmd/slowjs)
# for the five targets the engine runs on, with <version> stamped in, packs
# each binary with the license texts NOTICE.md says a binary must carry, and
# writes <dir>/checksums.txt. The release job (.github/workflows/release.yml)
# runs it, see docs/releasing.md. Each archive holds one folder,
# slowjs-<target>, named after the target as Rust names it, which is what
# installers of prebuilt binaries look for:
#
#   slowjs-x86_64-unknown-linux-gnu.tar.gz    linux/amd64
#   slowjs-aarch64-unknown-linux-gnu.tar.gz   linux/arm64
#   slowjs-x86_64-apple-darwin.tar.gz         darwin/amd64
#   slowjs-aarch64-apple-darwin.tar.gz        darwin/arm64
#   slowjs-x86_64-pc-windows-msvc.zip         windows/amd64
#
# The binaries are static: no cgo, no C library, so the linux ones run on
# glibc and musl systems alike, and the windows one needs no runtime.
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
# names. The binary links no Intl data, so the notices of intl/ and
# internal/gointl are not among them.
notices="NOTICE.md LICENSE LICENSE-MIT LICENSE-LIBQUICKJS LICENSE-MODERNC-LIBQUICKJS LICENSE-TLSF engine/libc/COPYRIGHT"
# What else a Go binary links: the runtime and standard library of Go, and
# golang.org/x/sys, the only module the command links besides this one.
goroot=$(go env GOROOT)
xsys=$(go mod download -json golang.org/x/sys | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')
xsysver=$(go list -m -f '{{.Version}}' golang.org/x/sys)
if [ ! -f "$goroot/LICENSE" ] || [ ! -f "$xsys/LICENSE" ]; then
	echo "$0: the LICENSE of Go ($goroot) or of golang.org/x/sys ($xsys) is missing" >&2
	exit 1
fi

for t in linux/amd64/x86_64-unknown-linux-gnu linux/arm64/aarch64-unknown-linux-gnu \
	darwin/amd64/x86_64-apple-darwin darwin/arm64/aarch64-apple-darwin \
	windows/amd64/x86_64-pc-windows-msvc; do
	goos=${t%%/*}
	rest=${t#*/}
	goarch=${rest%%/*}
	name=slowjs-${rest#*/}
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
