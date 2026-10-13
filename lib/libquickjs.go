// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run generator.go

// Package libquickjs is a pure Go embeddable Javascript engine. It supports
// the ECMA script 14 (ES2023) specification including modules, asynchronous
// generators, proxies and BigInt.
//
// This package is the ccgo transpilation of [QuickJS]. The idiomatic Go
// wrapper is available at [github.com/liustack/quickjs].
//
// # QuickJS Version
//
// Release 2026-06-04, [changelog].
//
// # Supported platforms and architectures
//
// These combinations of GOOS and GOARCH are currently supported
//
//	OS      Arch
//	---------------
//	darwin  amd64
//	darwin  arm64
//	freebsd amd64
//	freebsd arm64
//	linux   386
//	linux   amd64
//	linux   arm
//	linux   arm64
//	linux   loong64
//	linux   ppc64le
//	linux   riscv64
//	linux   s390x
//	windows amd64
//	windows arm64
//
// # Builders
//
// Builder results are available [here]:
//
// [QuickJS]: https://bellard.org/quickjs/
// [changelog]: https://bellard.org/quickjs/Changelog
// [here]: https://modern-c.appspot.com/-/builder/?importpath=modernc.org%2flibquickjs
// [github.com/liustack/quickjs]: https://pkg.go.dev/github.com/liustack/quickjs
package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

func init() {
	Xinit(nil)
}

const (
	// MaxStackSlots defines the default maximum allowed number of libc.TLS stack
	// slots before a stack overflow is detected.
	MaxStackSlots = 1000
	// 1, 2, 4, 8, 16: linux/amd64: nothing works
	// 32: linux/amd64: Some simple tests work
	// 64, 128: linux/amd64: Simple tests pass, some test262 tests work
	// 512: linux/amd64: All tests pass
)

func _js_check_stack_overflow(tls *libc.TLS, rt uintptr, sz Size_t) int32 {
	limit := (*TJSRuntime)(unsafe.Pointer(rt)).Fstack_size
	if limit == 0 {
		// Maintain backward compatibility. Also, some 262 tests fail with no limit.
		limit = MaxStackSlots
	}
	if Tuintptr_t(tls.StackSlots()) < limit {
		return 0
	}

	return 1
}

func _getcwd0(tls *libc.TLS, p uintptr, n Size_t) uintptr {
	return libc.Xgetcwd(tls, p, n)
}
