//go:build goasm && linux && amd64

// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package libquickjs

import (
	"modernc.org/libc"
	"runtime/debug"
	"strings"
	"testing"
	"unsafe"
)

func TestGoasmArgumentLayout(t *testing.T) {
	var a goasmArgs
	got := []uintptr{unsafe.Sizeof(a), unsafe.Offsetof(a.Ctx), unsafe.Offsetof(a.Fn), unsafe.Offsetof(a.This), unsafe.Offsetof(a.NewTarget), unsafe.Offsetof(a.Argc), unsafe.Offsetof(a.Argv), unsafe.Offsetof(a.Flags), unsafe.Offsetof(a.Result), unsafe.Offsetof(a.Allocated), unsafe.Offsetof(a.TLSSP), unsafe.Offsetof(a.TLSStack), unsafe.Offsetof(a.TLSSignals)}
	// 48 bytes of varargs follow goasmArgs, then the locals.
	want := []uintptr{136, 8, 16, 32, 48, 64, 72, 80, 88, 104, 112, 120, 128}
	if unsafe.Sizeof(a)+48 > goasmLocals {
		t.Fatalf("varargs overlap the locals at %d", goasmLocals)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("field %d: got %d want %d", i, got[i], want[i])
		}
	}
}
func TestGoasmNilFaultTrace(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	defer func() {
		if recover() == nil {
			t.Error("native nil access did not panic")
		}
		trace := string(debug.Stack())
		if !strings.Contains(trace, "goasmRun") {
			t.Fatalf("assembly absent from trace: %s", trace)
		}
	}()
	goasmRun(tls, 0, 0)
}
