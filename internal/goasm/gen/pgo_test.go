// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"reflect"
	"strings"
	"testing"
)

func TestStripCGProfile(t *testing.T) {
	in := `define void @f() {
  ret void
}
!llvm.module.flags = !{!0, !1, !7, !2}
!0 = !{i32 1, !"wchar_size", i32 4}
!1 = !{i32 8, !"PIC Level", i32 2}
!2 = !{i32 7, !"frame-pointer", i32 2}
!7 = !{i32 5, !"CG Profile", !8}
!8 = !{!9}
`
	want := `define void @f() {
  ret void
}
!llvm.module.flags = !{!0, !1, !2}
!0 = !{i32 1, !"wchar_size", i32 4}
!1 = !{i32 8, !"PIC Level", i32 2}
!2 = !{i32 7, !"frame-pointer", i32 2}
!8 = !{!9}
`
	if got := StripCGProfile(in); got != want {
		t.Fatalf("StripCGProfile:\n%s\nwant\n%s", got, want)
	}
	for _, s := range []string{want, "define void @g() {\n}\n"} {
		if got := StripCGProfile(s); got != s {
			t.Fatalf("a module without the flag changed:\n%s", got)
		}
	}
	last := strings.Replace(in, "!{!0, !1, !7, !2}", "!{!0, !1, !2, !7}", 1)
	if got := StripCGProfile(last); !strings.Contains(got, "!llvm.module.flags = !{!0, !1, !2}\n") {
		t.Fatalf("the flag last in the list stayed:\n%s", got)
	}
}

func TestInlineRemarks(t *testing.T) {
	in := `engine/quickjs.c:10:5: remark: 'f' inlined into 'g' with (cost=5, threshold=400) at callsite g:2:5; [-Rpass=inline]
   10 |     f();
      |     ^
engine/quickjs.c:12:5: remark: 'h' not inlined into 'g' because too costly to inline [-Rpass-missed=inline]
warning: something else
`
	want := "engine/quickjs.c:10:5: remark: 'f' inlined into 'g' with (cost=5, threshold=400) at callsite g:2:5; [-Rpass=inline]\n"
	if got := InlineRemarks(in); got != want {
		t.Fatalf("InlineRemarks = %q, want %q", got, want)
	}
}

func TestProfileNames(t *testing.T) {
	text := `JS_NewRuntime
# Func Hash:
1
# Num Counters:
1
# Counter Values:
3

quickjs.c:js_free_shape
# Func Hash:
2
# Num Counters:
1
# Counter Values:
0
`
	got := ProfileNames(text)
	want := map[string]bool{"JS_NewRuntime": true, "quickjs.c:js_free_shape": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProfileNames = %v, want %v", got, want)
	}
}

func TestUnprofiledFunctions(t *testing.T) {
	ir := `define void @js_free_rt(ptr %0) #0 !prof !62 {
define internal [2 x i64] @JS_RegvmRun(ptr noundef %0) #5 !PGOFuncName !2434 {
define internal i32 @rv_getel_atom(ptr %0) #5 !PGOFuncName !2655 {
define internal void @JS_FreeValueRT(ptr %0) #4 !prof !347 !PGOFuncName !348 {
define i32 @JS_NewThing(ptr %0) #0 {
declare void @host_trap()
!2434 = !{!"quickjs.c:JS_RegvmRun"}
!2655 = !{!"quickjs.c:rv_getel_atom"}
!348 = !{!"quickjs.c:JS_FreeValueRT"}
`
	profile := map[string]bool{"js_free_rt": true, "quickjs.c:JS_RegvmRun": true,
		"quickjs.c:JS_FreeValueRT": true, "JS_NewThing": true}
	stale, fresh := UnprofiledFunctions(ir, profile)
	if !reflect.DeepEqual(stale, []string{"JS_NewThing", "quickjs.c:JS_RegvmRun"}) {
		t.Fatalf("stale = %v", stale)
	}
	if !reflect.DeepEqual(fresh, []string{"quickjs.c:rv_getel_atom"}) {
		t.Fatalf("fresh = %v", fresh)
	}
}

func TestStaleWarnings(t *testing.T) {
	if w := StaleWarnings([]string{"quickjs.c:js_a", "js_b"}, []string{"js_new"}, []string{"JS_RegvmRun"}, 2); len(w) != 0 {
		t.Fatalf("two stale functions of at most two and a new one: %q", w)
	}
	for _, c := range []struct{ stale, fresh []string }{
		{[]string{"quickjs.c:JS_RegvmRun"}, nil},
		{nil, []string{"JS_RegvmRun"}},
	} {
		w := StaleWarnings(c.stale, c.fresh, []string{"JS_RegvmRun"}, 30)
		if len(w) != 1 || !strings.Contains(w[0], "JS_RegvmRun has no profile") {
			t.Fatalf("stale %q new %q: %q", c.stale, c.fresh, w)
		}
	}
	w := StaleWarnings([]string{"a", "b", "c"}, nil, []string{"JS_RegvmRun"}, 2)
	if len(w) != 1 || !strings.Contains(w[0], "3 functions") {
		t.Fatalf("three stale functions of at most two: %q", w)
	}
}

func TestMisexpectLines(t *testing.T) {
	in := "engine/a.c:3:9: warning: potential performance regression from use of __builtin_expect(): annotation was correct on 2.00% (1 / 50) of profiled executions [-Wmisexpect]\n" +
		"    3 |     if (unlikely(x))\n" +
		"engine/a.c:7:1: warning: unused variable 'y' [-Wunused-variable]\n"
	got := MisexpectLines(in)
	if len(got) != 1 || !strings.HasPrefix(got[0], "engine/a.c:3:9:") {
		t.Fatalf("MisexpectLines = %q, want the one -Wmisexpect warning", got)
	}
}
