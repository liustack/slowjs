// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"reflect"
	"testing"
)

const head = `@table = internal constant [1 x ptr] [ptr @target]
`

const tail = `!1 = !{i32 111}
!2 = !{i32 222}
`

func module(body string) string { return head + body + tail }

func cyclesOf(t *testing.T, ir string) [][]string {
	t.Helper()
	got, err := cycles(ir)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A cycle that no count cuts is found, through direct and indirect calls, and
// only those.
func TestCycles(t *testing.T) {
	cutAB := func(count string) string {
		return module(`define internal void @a() {
entry:
  %call = call i32 @` + count + `(ptr noundef null)
  call void @b()
  ret void
}
define internal void @b() {
entry:
  call void @a()
  ret void
}
`)
	}
	indirect := func(typ string) string {
		return module(`define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  %0 = load ptr, ptr %f
  call void %0(ptr noundef %f) [ "kcfi"(i32 ` + typ + `) ]
  ret void
}
`)
	}
	for _, c := range []struct {
		name string
		ir   string
		want [][]string
	}{
		{"self call", module(`define internal void @rec(i32 noundef %n) {
entry:
  call void @rec(i32 noundef 1)
  ret void
}
`), [][]string{{"rec"}}},
		{"count cuts", cutAB("js_internal_enter"), [][]string{}},
		{"json count cuts", cutAB("js_json_enter"), [][]string{}},
		{"two functions", module(`define internal { i64, i64 } @a() {
entry:
  %call = call { i64, i64 } @b()
  ret { i64, i64 } %call
}
define internal { i64, i64 } @b() {
entry:
  %call = call { i64, i64 } @a()
  ret { i64, i64 } %call
}
`), [][]string{{"a", "b"}}},
		{"indirect call by type", indirect("111"), [][]string{{"target"}}},
		{"indirect call of another type", indirect("222"), [][]string{}},
		{"address not taken", `define internal void @f(ptr noundef %p) !kcfi_type !1 {
entry:
  %0 = load ptr, ptr %p
  call void %0(ptr noundef %p) [ "kcfi"(i32 111) ]
  ret void
}
!1 = !{i32 111}
`, [][]string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := cyclesOf(t, c.ir); !reflect.DeepEqual(got, c.want) {
				t.Errorf("cycles = %v, want %v", got, c.want)
			}
		})
	}
}

// The largest cycle comes first, and a cycle of the same size by name.
func TestCyclesOrder(t *testing.T) {
	ir := module(`define internal void @z() {
entry:
  call void @z()
  ret void
}
define internal void @a() {
entry:
  call void @a()
  ret void
}
define internal void @m() {
entry:
  call void @n()
  ret void
}
define internal void @n() {
entry:
  call void @m()
  ret void
}
`)
	want := [][]string{{"m", "n"}, {"a"}, {"z"}}
	if got := cyclesOf(t, ir); !reflect.DeepEqual(got, want) {
		t.Errorf("cycles = %v, want %v", got, want)
	}
}

// A call to the pair's second function is found through direct calls, typed
// indirect calls and counts. A count does not cut a path.
func TestReachDirectAndIndirect(t *testing.T) {
	ir := module(`define internal void @a() {
entry:
  call void @b()
  ret void
}
define internal void @b() {
entry:
  %0 = load ptr, ptr @table
  call void %0(ptr noundef null) [ "kcfi"(i32 111) ]
  ret void
}
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  %call = call i32 @js_internal_enter(ptr noundef null)
  call void @c()
  ret void
}
define internal void @c() {
entry:
  ret void
}
`)
	got, err := reaches(ir, []pair{{"a", "c"}, {"c", "a"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []reach{{"a", "c", []string{"a", "b", "target", "c"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reaches = %v, want %v", got, want)
	}
}

// A pair that names a function the module does not define checks nothing, as
// after a rename: it is an error.
func TestReachUnknownFunction(t *testing.T) {
	ir := module(`define internal void @a() {
entry:
  ret void
}
`)
	for _, p := range []pair{{"a", "missing"}, {"missing", "a"}} {
		if _, err := reaches(ir, []pair{p}); err == nil {
			t.Errorf("reaches %v: no error", p)
		}
	}
}

func TestReachOtherType(t *testing.T) {
	ir := module(`define internal void @a() {
entry:
  %0 = load ptr, ptr @table
  call void %0(ptr noundef null) [ "kcfi"(i32 222) ]
  ret void
}
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  ret void
}
`)
	got, err := reaches(ir, []pair{{"a", "target"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("reaches = %v, want none", got)
	}
}

// An indirect call whose kcfi type is missing cannot be joined to its targets,
// so it is an error and not a silent gap in the graph.
func TestUntypedIndirectCall(t *testing.T) {
	ir := module(`define internal void @f(ptr noundef %p) {
entry:
  call void %p()
  ret void
}
`)
	if _, err := cycles(ir); err == nil {
		t.Error("an indirect call with no kcfi type: no error")
	}
}
