// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"
)

// TestCheckPointerTargets: a pointer the code takes or engine_rodata holds
// may name a lowered unified function, a stub or an alias of one, and the
// generator refuses every other: a host callback such as host_now, the
// generator's callback, a register-entry leaf, an unknown name, and an
// address inside a function.
func TestCheckPointerTargets(t *testing.T) {
	takes := func(name string, add int64) Meta {
		return Meta{Name: "f", Kind: Unified, Rec: &FuncRecord{Name: "f", Refs: []RefRecord{{Label: name, Add: add, Func: true}}}}
	}
	base := []Meta{{Name: "u", Kind: Unified}, {Name: "lf", Kind: Register}}
	stubs := []Meta{{Name: "s", Kind: Unified}}
	sets := map[string]string{"weak_u": "u", "weak_weak_u": "weak_u", "weak_lf": "lf"}
	holds := func(name string, add int64) *DataPlan {
		return &DataPlan{Relocs: []BlobReloc{{Off: 8, Sym: name, Add: add, Func: true}}}
	}
	for _, name := range []string{"u", "s", "weak_u", "weak_weak_u", "f"} {
		if err := CheckPointerTargets(append([]Meta{takes(name, 0)}, base...), stubs, nil, sets); err != nil {
			t.Errorf("the code takes the address of %s: %v", name, err)
		}
		if err := CheckPointerTargets(base, stubs, holds(name, 0), sets); err != nil && name != "f" {
			t.Errorf("engine_rodata holds %s: %v", name, err)
		}
	}
	for _, c := range []struct {
		name string
		add  int64
		want string
	}{
		{"host_now", 0, "host_now, which is a host callback"},
		{"host_call", 0, "host_call, which is a host callback"},
		{"host_trap", 0, "host_trap, which is a callback of the generator"},
		{"lf", 0, "lf, which is a register-entry leaf"},
		{"weak_lf", 0, "weak_lf, which is a register-entry leaf"},
		{"nothing", 0, "nothing, which is no generated function"},
		{"u", 16, "u+16, inside a function"},
	} {
		err := CheckPointerTargets(append([]Meta{takes(c.name, c.add)}, base...), stubs, nil, sets)
		if err == nil || !strings.Contains(err.Error(), "f holds the address of "+c.want) {
			t.Errorf("the code takes the address of %s%+d: %v, want %q", c.name, c.add, err, c.want)
		}
		err = CheckPointerTargets(base, stubs, holds(c.name, c.add), sets)
		if err == nil || !strings.Contains(err.Error(), "engine_rodata at 8 holds the address of "+c.want) {
			t.Errorf("engine_rodata holds %s%+d: %v, want %q", c.name, c.add, err, c.want)
		}
	}
}
