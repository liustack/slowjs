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

// noSplitMetas is a small engine for MarkNoSplit: leaves of each size on
// both sides of what the assembler marks by itself, functions that call
// directly and through a pointer, and a register-entry leaf.
func noSplitMetas() []Meta {
	u := func(name string, frame int, calls ...CallRecord) Meta {
		return Meta{Name: name, Kind: Unified, Frame: frame, ArgBytes: 8, Rec: &FuncRecord{Name: name, Calls: calls}}
	}
	call := CallRecord{Kind: CallUnified, Target: "tiny"}
	return []Meta{
		u("tiny", 0),
		u("small", 112), // 120 with the base pointer, 128 on arm64
		u("big", 120),   // 128 with the base pointer
		u("calls", 0, call),
		u("icall", 16, CallRecord{Kind: CallReg, Target: "rax"}),
		u("copies", 16, CallRecord{Kind: CallLeaf, Target: "memcpy"}),
		{Name: "memcpy", Kind: Register, Frame: 8, Rec: &FuncRecord{Name: "memcpy", Kind: Register}},
		reg("wrap", 16, "memcpy"),
	}
}

// reg is a register-entry leaf that calls the register-entry leaves calls.
func reg(name string, frame int, calls ...string) Meta {
	rec := &FuncRecord{Name: name, Kind: Register}
	for _, c := range calls {
		rec.Calls = append(rec.Calls, CallRecord{Kind: CallLeaf, Target: c})
	}
	return Meta{Name: name, Kind: Register, Frame: frame, Rec: rec}
}

// TestMarkNoSplit: the list is the register-entry leaves and the leaves
// whose frame the assembler marks by itself, by the frame of each
// architecture, and nothing that calls. Meta.NoSplit and the record's agree
// with it, also where they said otherwise before.
func TestMarkNoSplit(t *testing.T) {
	for _, c := range []struct {
		r    StackRules
		auto []string
	}{
		{AMD64Stack, []string{"tiny", "small"}},
		{ARM64Stack, []string{"tiny"}},
	} {
		ms := noSplitMetas()
		ms[2].NoSplit, ms[2].Rec.NoSplit = true, true // big, said NOSPLIT before
		l, err := MarkNoSplit(c.r, ms)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(l.Auto, c.auto) {
			t.Errorf("%s: auto leaves %v, want %v", c.r.Arch, l.Auto, c.auto)
		}
		if want := []string{"memcpy", "wrap"}; !reflect.DeepEqual(l.Register, want) {
			t.Errorf("%s: register-entry leaves %v, want %v", c.r.Arch, l.Register, want)
		}
		for _, m := range ms {
			if m.NoSplit != l.Set[m.Name] || m.Rec.NoSplit != m.NoSplit {
				t.Errorf("%s: %s NoSplit %v, record %v, on the list %v", c.r.Arch, m.Name, m.NoSplit, m.Rec.NoSplit, l.Set[m.Name])
			}
		}
		if len(l.Set) != len(c.auto)+2 {
			t.Errorf("%s: the list %v", c.r.Arch, l.Set)
		}
		r := NoSplitReport(l, ms)
		if !strings.Contains(r, "tiny auto frame 0 args 8\n") || !strings.Contains(r, "memcpy register frame 8 args 0\n") || strings.Contains(r, "calls ") {
			t.Errorf("%s: the report:\n%s", c.r.Arch, r)
		}
	}
}

// TestMarkNoSplitErrors: a register-entry leaf that calls a unified
// function, which would start a chain without a stack check, leaves that
// call each other in a cycle, a chain of leaves that takes 128 bytes, and a
// function given twice are errors. A chain of 120 bytes is not (a frame
// with calls takes 8 bytes for the base pointer, and each call 8).
func TestMarkNoSplitErrors(t *testing.T) {
	for _, c := range []struct {
		name  string
		metas []Meta
		want  string
	}{
		{"calls a unified function", []Meta{
			{Name: "tiny", Kind: Unified, Rec: &FuncRecord{Name: "tiny"}},
			{Name: "leaf", Kind: Register, Rec: &FuncRecord{Name: "leaf", Kind: Register, Calls: []CallRecord{{Kind: CallUnified, Target: "tiny"}}}},
		}, "leaf leaf calls tiny, which is not a register-entry leaf"},
		{"calls an unknown leaf", []Meta{reg("leaf", 0, "nowhere")}, "calls nowhere"},
		{"a cycle", []Meta{reg("a", 0, "b"), reg("b", 0, "c"), reg("c", 0, "a")}, "calls itself through other leaves"},
		{"too tall", []Meta{reg("a", 0, "b"), reg("b", 40, "c"), reg("c", 48)}, "leaf a takes 128 bytes"},
		{"just fits", []Meta{reg("a", 0, "b"), reg("b", 40, "c"), reg("c", 40)}, ""},
		{"twice", []Meta{reg("a", 0), reg("a", 0)}, "a twice"},
	} {
		_, err := MarkNoSplit(AMD64Stack, c.metas)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}
