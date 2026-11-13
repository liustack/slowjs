// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"sort"
	"strings"
)

// CheckPointerTargets holds every function a pointer of the engine can hold
// to the one kind a call through a pointer may reach: a lowered unified
// function, or a stub, which never returns (it calls goasmMissing, which
// panics). A pointer comes from the code, which takes the address of a
// function (RefRecord.Func), or from engine_rodata, which holds it
// (BlobReloc.Func). A call through a pointer marshals its arguments as a
// unified call and takes its result from the registers a lowered unified
// function returns it in (CallRecord.RegResult), so a pointer to a host
// callback (Go code, which returns its result on the Go stack), a
// register-entry leaf or anything else would be called wrongly. sets are the
// unit's symbol assignments (AsmUnit.Sets): a pointer to an alias is a
// pointer to the function it names. A pointer into a function (an addend)
// is refused too. The post-link checks hold the linked binary to the same
// rule (checker.pointerTarget, a64Checker.pointerTarget).
func CheckPointerTargets(metas, stubs []Meta, plan *DataPlan, sets map[string]string) error {
	ok := map[string]bool{}
	what := map[string]string{}
	for _, m := range metas {
		if m.Kind == Unified {
			ok[m.Name] = true
		} else {
			what[m.Name] = "a register-entry leaf"
		}
	}
	for _, m := range stubs {
		ok[m.Name] = true
	}
	for _, h := range HostCallbacks {
		what[h] = "a host callback (Go code)"
	}
	for _, h := range GeneratorCallbacks {
		what[h] = "a callback of the generator (Go code)"
	}
	var bad []string
	check := func(where, name string, add int64) {
		t := name
		for range len(sets) + 1 {
			n, isSet := sets[t]
			if !isSet {
				break
			}
			t = n
		}
		switch {
		case add != 0:
			bad = append(bad, fmt.Sprintf("%s holds the address of %s%+d, inside a function", where, name, add))
		case !ok[t]:
			w := what[t]
			if w == "" {
				w = "no generated function"
			}
			bad = append(bad, fmt.Sprintf("%s holds the address of %s, which is %s", where, name, w))
		}
	}
	for _, m := range metas {
		if m.Rec == nil {
			continue
		}
		for _, r := range m.Rec.Refs {
			if r.Func {
				check(m.Name, r.Label, r.Add)
			}
		}
	}
	if plan != nil {
		for _, r := range plan.Relocs {
			if r.Func {
				check(fmt.Sprintf("engine_rodata at %d", r.Off), r.Sym, r.Add)
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("a call through a pointer reaches only a lowered unified function, and %d pointers would reach something else: %s",
		len(bad), strings.Join(bad, "; "))
}
