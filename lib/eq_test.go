// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// testdata/eq.js runs ===, !==, == and != on every pair of kinds of
// values. testdata/eq.txt is the transcript of the engine before fixEq
// (Node gives the same), so it must stay as it is.
func TestEqSemantics(t *testing.T) {
	matchTranscript(t, "eq")
}

// Comparing values made for the comparison frees each of them once: after
// many rounds, as many objects, strings and atoms are alive as after one.
func TestEqFrees(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	if got := eval1(t, tls, ctx, `
var keep = {}, ksym = Symbol('k');
function fresh(i) {
	switch (i % 8) {
	case 0: return {};
	case 1: return 'x'.repeat(3) + i;
	case 2: return Symbol('s' + i);
	case 3: return 'x'.repeat(400) + i;
	case 4: return null;
	case 5: return Symbol('d' + i).description;
	case 6: return [i];
	default: return 'k' + (i % 3);
	}
}
function run(n) {
	var c = 0;
	for (var i = 0; i < n; i++) {
		var a = fresh(i), b = fresh(i + (i & 1)), s = 'xxx' + i;
		if (fresh(i) === fresh(i + 1)) c++;
		if (fresh(i) !== b) c++;
		if (null != fresh(i)) c++;
		if (fresh(i) == void 0) c++;
		if (a === a) c++;
		if (a == b) c++;
		if (s === 'xxx' + i) c++;
		if (keep === fresh(i)) c++;
		if (ksym !== fresh(i)) c++;
	}
	return c;
}
run(8)`); got == "" {
		t.Fatal("no result")
	}
	count := func(rounds string) (objs, strs, atoms int64) {
		eval1(t, tls, ctx, "run("+rounds+")")
		XJS_RunGC(tls, rt)
		var u TJSMemoryUsage
		XJS_ComputeMemoryUsage(tls, rt, uintptr(unsafe.Pointer(&u)))
		return u.Fobj_count, u.Fstr_count, u.Fatom_count
	}
	o1, s1, a1 := count("8")
	o2, s2, a2 := count("8000")
	if o1 != o2 || s1 != s2 || a1 != a2 {
		t.Errorf("after 8 rounds %d objects, %d strings and %d atoms, after 8000 %d, %d and %d", o1, s1, a1, o2, s2, a2)
	}
}
