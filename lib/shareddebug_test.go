// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// readFunction compiles js, writes it as bytecode and reads it back in a new
// runtime with flags. It returns the runtime, the bytecode and the function f
// of js, read from the constant pool of the script.
func readFunction(t *testing.T, tls *libc.TLS, js string, flags int32) (rt, ctx uintptr, buf []byte, p uintptr, script, f TJSValue) {
	t.Helper()
	crt := XJS_NewRuntime(tls)
	cctx := XJS_NewContext(tls, crt)
	cs, err := libc.CString(js)
	if err != nil {
		t.Fatal(err)
	}
	compiled := XJS_Eval(tls, cctx, cs, Size_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), MJS_EVAL_TYPE_GLOBAL|MJS_EVAL_FLAG_COMPILE_ONLY)
	libc.Xfree(tls, cs)
	if tag(compiled) != EJS_TAG_FUNCTION_BYTECODE {
		t.Fatalf("compile: tag %d", tag(compiled))
	}
	plen := tls.Alloc(8)
	out := XJS_WriteObject(tls, cctx, plen, compiled, MJS_WRITE_OBJ_BYTECODE)
	n := int(*(*Size_t)(unsafe.Pointer(plen)))
	tls.Free(8)
	buf = append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(out)), n)...)
	Xjs_free(tls, cctx, out)
	XFreeValue(tls, cctx, compiled)
	XJS_FreeContext(tls, cctx)
	XJS_FreeRuntime(tls, crt)

	rt = XJS_NewRuntime(tls)
	ctx = XJS_NewContext(tls, rt)
	p = libc.Xmalloc(tls, Size_t(n))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), n), buf)
	script = XJS_ReadObject(tls, ctx, p, Size_t(n), MJS_READ_OBJ_BYTECODE|flags)
	if tag(script) != EJS_TAG_FUNCTION_BYTECODE {
		t.Fatalf("read: tag %d", tag(script))
	}
	b := (*TJSFunctionBytecode)(unsafe.Pointer(jsvPtr(script)))
	if b.Fcpool_count != 1 {
		t.Fatalf("the script has %d constants, want f alone", b.Fcpool_count)
	}
	f = *(*TJSValue)(unsafe.Pointer(b.Fcpool))
	return rt, ctx, buf, p, script, f
}

func memoryUsage(tls *libc.TLS, rt uintptr) TJSMemoryUsage {
	p := tls.Alloc(int(unsafe.Sizeof(TJSMemoryUsage{})))
	defer tls.Free(int(unsafe.Sizeof(TJSMemoryUsage{})))
	XJS_ComputeMemoryUsage(tls, rt, p)
	return *(*TJSMemoryUsage)(unsafe.Pointer(p))
}

// With ReadObjSharedDebug the source text and line table of a function point
// into the buffer read, the function says so, the runtime neither counts nor
// frees them, and they hold what a copy holds.
func TestSharedDebugRead(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	js := "function f(a) {\n\t/*" + strings.Repeat("y", 64<<10) + "*/\n\treturn new Error(a).stack\n}\n"
	var sources [2]string
	var usage [2]TJSMemoryUsage
	for i, flags := range []int32{0, ReadObjSharedDebug} {
		rt, ctx, buf, p, script, f := readFunction(t, tls, js, flags)
		b := (*TJSFunctionBytecode)(unsafe.Pointer(jsvPtr(f)))
		src, pc := b.Fdebug.Fsource, b.Fdebug.Fpc2line_buf
		inside := func(q uintptr) bool { return q >= p && q < p+uintptr(len(buf)) }
		shared := flags != 0
		if inside(src) != shared || inside(pc) != shared || (b.F__ccgo18&sharedDebugBit != 0) != shared {
			t.Errorf("flags %d: source in the buffer %v, line table %v, bit %#x", flags, inside(src), inside(pc), b.F__ccgo18)
		}
		sources[i] = string(unsafe.Slice((*byte)(unsafe.Pointer(src)), b.Fdebug.Fsource_len))
		usage[i] = memoryUsage(tls, rt)
		// Freeing the functions must leave the buffer to its owner.
		XFreeValue(tls, ctx, script)
		XJS_FreeContext(tls, ctx)
		XJS_FreeRuntime(tls, rt)
		libc.Xfree(tls, p)
	}
	if sources[0] != sources[1] || !strings.HasPrefix(sources[0], "function f(a) {") {
		t.Errorf("source %.40q, shared %.40q", sources[0], sources[1])
	}
	if got := usage[1].Fjs_func_pc2line_size; got != 0 {
		t.Errorf("the runtime counts %d bytes of shared line tables", got)
	}
	if saved := usage[0].Fmalloc_size - usage[1].Fmalloc_size; saved < 64<<10 {
		t.Errorf("shared, the runtime takes %d bytes less, want at least %d", saved, 64<<10)
	}
	if saved := usage[0].Fjs_func_size - usage[1].Fjs_func_size; saved < 64<<10 {
		t.Errorf("shared, the runtime counts %d bytes less in its functions, want at least %d", saved, 64<<10)
	}
}
