// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions without Go's stack check. Every unified entry starts with the
// stack check of the Go assembler, three instructions that compare the stack
// pointer with the guard in g and call morestack below it. A function whose
// TEXT says NOSPLIT has none: the Go documentation of the assembler gives it
// the room left above the guard, and the linker adds up every chain of
// NOSPLIT calls and fails the build when one needs more than that room
// (cmd/link/internal/ld/stackcheck.go: 800 bytes less one CALL on amd64,
// "nosplit stack over 792 byte limit"). The Go assembler marks a function
// NOSPLIT on its own when it calls nothing and its frame with the saved base
// pointer is under 128 bytes.
//
// PickNoSplit chooses the functions whose TEXT says NOSPLIT. The list holds
// the leaves the assembler would mark anyway, named so that the post-link
// check knows every function without a stack check, the register-entry
// leaves, which are NOSPLIT by their form, and the functions it chooses on
// top of those. A chosen function
//
//   - has no address taken, so that no call through a pointer reaches it: the
//     linker takes the target of an indirect call for a function with a
//     stack check and counts only the CALL,
//   - makes no call through a pointer, so that every function a chain of
//     NOSPLIT calls reaches is one the linker counts (an automatic NOSPLIT
//     leaf whose address is taken is then reached only from functions with
//     a stack check),
//   - is on no cycle of direct calls, so that a recursion always goes through
//     a stack check, which grows the stack and is where the Go runtime can
//     stop the goroutine,
//   - and fits the room, with what a fault and a Go caller add to it (below).
//
// A chain of NOSPLIT calls starts below a function with a stack check, which
// leaves at least 800 bytes above the bottom of the goroutine's stack once
// its frame is made (internal/abi.StackNosplitBase, which the runtime's guard
// adds to the 128 bytes of abi.StackSmall). The room a chain may take is
// counted as the linker counts it: each frame with its saved base pointer,
// 8 bytes for each CALL, 8 for the CALL of morestack of a function with a
// stack check that a chain calls, and the height of each NOSPLIT callee. Two
// things the linker does not count are added:
//
//   - A fault in a NOSPLIT function (a wild pointer of the C) becomes a call
//     of runtime.sigpanic that the signal handler pushes onto the
//     goroutine's stack where the fault is (runtime/signal_amd64.go,
//     pushCall), and sigpanic has a stack check that may call morestack
//     (runtime/signal_unix.go: it must not be nosplit "because the linker
//     doesn't know where sigpanic calls can be injected"). morestack needs
//     those two return addresses, 16 bytes, to fit above the bottom of the
//     stack: newstack throws "split stack overflow", which ends the process,
//     when they do not. FaultRoom is kept below the deepest frame of every
//     chain for this.
//   - A Go func value of an assembly function calls it through an ABI
//     wrapper that the compiler marks NOSPLIT (cmd/compile/internal/ssagen/
//     abi.go), with a frame for the function's arguments: the linker counts
//     the arguments, the saved base pointer and the CALL above the function,
//     ArgBytes + 16 (a goasmkeep build takes the value of every function).
//     Each chosen function leaves that room above it.
//
// Go host callbacks are reached through such a wrapper too, the other way
// round, and are counted with CallbackHeight (the linker counted at most 72
// bytes for those of internal/cengine: the wrapper of host_call).
//
// The choice goes from the callees up: a function is taken when its chain
// fits with the callees taken so far, and taking it changes only the
// functions that call it, which come later. Nothing depends on how often a
// function is called, so the same C gives the same list.
package gen

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// NoSplitLimit is the room the linker gives a chain of NOSPLIT calls on
	// amd64: StackNosplit (800) less the CALL into the chain.
	NoSplitLimit = 792
	// FaultRoom is kept below the deepest frame of a chosen chain for the
	// call of sigpanic that a fault adds, and its call of morestack: 16
	// bytes, and as much again four times over in case a runtime needs more.
	FaultRoom = 64
	// CallbackHeight is what a call of a Go host callback adds below the
	// CALL: its ABI wrapper and the CALL of morestack of the Go function.
	CallbackHeight = 128
	// autoNoSplitFrame is the frame, with the saved base pointer, under
	// which the Go assembler marks a function that calls nothing NOSPLIT
	// (abi.StackSmall, cmd/internal/obj/x86/obj6.go).
	autoNoSplitFrame = 128
)

// NoSplitFunc is what the choice reads of one generated function.
type NoSplitFunc struct {
	Name     string
	Kind     Kind
	Frame    int      // the Go TEXT frame, without the saved base pointer
	ArgBytes int      // the Go TEXT argument and result area
	Calls    []string // direct call targets, in order, repeats allowed
	Indirect bool     // calls through a register
	Stub     bool     // a stub of a function the generator could not take
}

// NoSplitPick is the choice: the functions without a stack check, and why.
type NoSplitPick struct {
	// Set holds every function whose TEXT says NOSPLIT.
	Set map[string]bool
	// Auto are the leaves the assembler marks NOSPLIT by itself, Register
	// the register-entry leaves, Chosen the others in Set.
	Auto, Register, Chosen []string
	// Height is the room each function of Set takes below its entry,
	// FaultRoom included, as the choice counts it.
	Height map[string]int
	// Left are the functions that could not be chosen, by reason.
	Left map[string][]string
}

// local is the bytes a function's own frame takes below its entry: the
// frame and the base pointer the Go assembler saves, which it leaves out
// for a function with no frame that calls nothing.
func (f *NoSplitFunc) local() int {
	if f.Frame == 0 && len(f.Calls) == 0 && !f.Indirect {
		return 0
	}
	return f.Frame + 8
}

// autoNoSplit says whether the Go assembler marks f NOSPLIT by itself.
func (f *NoSplitFunc) autoNoSplit() bool {
	return f.Kind == Unified && !f.Stub && len(f.Calls) == 0 && !f.Indirect && f.local() < autoNoSplitFrame
}

// PickNoSplit chooses the NOSPLIT functions of funcs. taken lists the
// functions whose address is used other than by a direct call, callbacks the
// Go functions the engine calls. force adds functions whatever the rules say,
// for the negative build that shows the linker refuses a recursion. It
// fails when a call goes to a function it does not know, or when the
// choice breaks its own rules.
func PickNoSplit(funcs []NoSplitFunc, taken, callbacks map[string]bool, force []string) (*NoSplitPick, error) {
	by := map[string]*NoSplitFunc{}
	for i := range funcs {
		f := &funcs[i]
		if by[f.Name] != nil {
			return nil, fmt.Errorf("nosplit: %s twice", f.Name)
		}
		by[f.Name] = f
	}
	for _, f := range funcs {
		for _, c := range f.Calls {
			if by[c] == nil && !callbacks[c] {
				return nil, fmt.Errorf("nosplit: %s calls %s, which is neither generated nor a host callback", f.Name, c)
			}
		}
	}
	p := &NoSplitPick{Set: map[string]bool{}, Height: map[string]int{}, Left: map[string][]string{}}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		switch f := by[n]; {
		case f.Kind == Register:
			p.Set[n] = true
			p.Register = append(p.Register, n)
		case f.autoNoSplit():
			p.Set[n] = true
			p.Auto = append(p.Auto, n)
		}
	}
	cyclic := onCycle(names, by)
	// height is the room f takes below its entry with the set as it is.
	// A NOSPLIT function never reaches itself: a leaf calls nothing, and a
	// chosen function is on no cycle.
	memo := map[string]int{}
	var height func(n string) int
	height = func(n string) int {
		if h, ok := memo[n]; ok {
			return h
		}
		f := by[n]
		l := f.local()
		h := l + FaultRoom
		for _, c := range f.Calls {
			var below int
			switch {
			case p.Set[c]:
				below = height(c)
			case by[c] != nil:
				below = 8 // the CALL of morestack in its stack check
			default: // a Go callback
				below = CallbackHeight
			}
			h = max(h, l+8+below)
		}
		memo[n] = h
		return h
	}
	fits := func(f *NoSplitFunc) bool { return height(f.Name)+f.ArgBytes+16 <= NoSplitLimit }
	for _, n := range calleesFirst(names, by) {
		f := by[n]
		if p.Set[n] {
			continue
		}
		var why string
		switch {
		case f.Stub:
			why = "stub"
		case taken[n]:
			why = "address taken"
		case f.Indirect:
			why = "calls through a pointer"
		case cyclic[n]:
			why = "on a cycle of calls"
		}
		if why == "" {
			p.Set[n] = true
			if !fits(f) {
				delete(p.Set, n)
				why = "over the room"
			}
			// Taking f or not changes no height known so far: only the
			// functions that call f, which come later, depend on it.
			delete(memo, n)
		}
		if why != "" {
			p.Left[why] = append(p.Left[why], n)
			continue
		}
		p.Chosen = append(p.Chosen, n)
	}
	forced := map[string]bool{}
	for _, n := range force {
		if by[n] == nil {
			return nil, fmt.Errorf("nosplit: forced %s is not a generated function", n)
		}
		if !p.Set[n] {
			p.Set[n] = true
			p.Chosen = append(p.Chosen, n)
			forced[n] = true
		}
	}
	sort.Strings(p.Chosen)
	if len(forced) > 0 {
		// A negative build: what was forced may recurse, and only the
		// linker judges it.
		return p, nil
	}
	// The rules again, on the final set.
	memo = map[string]int{}
	for _, n := range names {
		if !p.Set[n] {
			continue
		}
		f := by[n]
		if f.Kind == Unified && !f.autoNoSplit() {
			if taken[n] || f.Indirect || cyclic[n] || f.Stub {
				return nil, fmt.Errorf("nosplit: %s was chosen against the rules", n)
			}
			if !fits(f) {
				return nil, fmt.Errorf("nosplit: %s takes %d bytes with its arguments, over %d", n, height(n)+f.ArgBytes+16, NoSplitLimit)
			}
		}
		p.Height[n] = height(n)
	}
	return p, nil
}

// onCycle reports the functions on a cycle of direct calls (Tarjan).
func onCycle(names []string, by map[string]*NoSplitFunc) map[string]bool {
	index, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	var stack []string
	out := map[string]bool{}
	next := 0
	var visit func(n string)
	visit = func(n string) {
		index[n], low[n] = next, next
		next++
		stack = append(stack, n)
		on[n] = true
		for _, c := range by[n].Calls {
			if by[c] == nil {
				continue
			}
			if c == n {
				out[n] = true
			}
			if _, seen := index[c]; !seen {
				visit(c)
				low[n] = min(low[n], low[c])
			} else if on[c] {
				low[n] = min(low[n], index[c])
			}
		}
		if low[n] == index[n] {
			var scc []string
			for {
				m := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[m] = false
				scc = append(scc, m)
				if m == n {
					break
				}
			}
			if len(scc) > 1 {
				for _, m := range scc {
					out[m] = true
				}
			}
		}
	}
	for _, n := range names {
		if _, seen := index[n]; !seen {
			visit(n)
		}
	}
	return out
}

// calleesFirst orders the functions so that each comes after the functions
// it calls, but for the calls of a cycle: a depth-first walk from each name
// in order, writing a function down once its callees are.
func calleesFirst(names []string, by map[string]*NoSplitFunc) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(n string)
	walk = func(n string) {
		seen[n] = true
		cs := append([]string{}, by[n].Calls...)
		sort.Strings(cs)
		for _, c := range cs {
			if by[c] != nil && !seen[c] {
				walk(c)
			}
		}
		out = append(out, n)
	}
	for _, n := range names {
		if !seen[n] {
			walk(n)
		}
	}
	return out
}

// ChooseNoSplit runs PickNoSplit on the lowered functions and the stubs and
// marks the chosen ones in their Meta and its record, and returns what it
// read of them. A function's address is taken when addrTaken (the pass's
// report) says so, when code loads it and when engine_rodata holds it. The
// calls of a function name their targets as llc's text does, an alias
// (unit.Sets) going to the function it names.
func ChooseNoSplit(metas, stubs []Meta, addrTaken map[string]bool, plan *DataPlan, unit *AsmUnit, callbacks map[string]bool, force []string) (*NoSplitPick, []NoSplitFunc, error) {
	resolve := func(n string) string {
		for i := 0; i < 8; i++ {
			t, ok := unit.Sets[n]
			if !ok {
				break
			}
			n = t
		}
		return n
	}
	taken := map[string]bool{}
	mark := func(n string) {
		for i := 0; i < 8 && n != ""; i++ {
			taken[n] = true
			n = unit.Sets[n]
		}
	}
	for n := range addrTaken {
		mark(n)
	}
	var fs []NoSplitFunc
	for _, m := range metas {
		f := NoSplitFunc{Name: m.Name, Kind: m.Kind, Frame: m.Frame, ArgBytes: m.ArgBytes}
		for _, c := range m.Rec.Calls {
			if c.Kind == CallReg {
				f.Indirect = true
				continue
			}
			f.Calls = append(f.Calls, resolve(c.Target))
		}
		for _, r := range m.Rec.Refs {
			if r.Func {
				mark(r.Label)
			}
		}
		fs = append(fs, f)
	}
	for _, m := range stubs {
		fs = append(fs, NoSplitFunc{Name: m.Name, Kind: Unified, ArgBytes: m.ArgBytes, Stub: true})
	}
	for _, r := range plan.Relocs {
		if r.Func {
			mark(r.Sym)
		}
	}
	p, err := PickNoSplit(fs, taken, callbacks, force)
	if err != nil {
		return nil, nil, err
	}
	for i := range metas {
		metas[i].NoSplit = p.Set[metas[i].Name]
		metas[i].Rec.NoSplit = metas[i].NoSplit
	}
	return p, fs, nil
}

// NoSplitReport is the list as the generator writes it down: one line per
// function, its kind (auto, register or chosen), frame and height.
func NoSplitReport(p *NoSplitPick, funcs []NoSplitFunc) string {
	kind := map[string]string{}
	for _, n := range p.Auto {
		kind[n] = "auto"
	}
	for _, n := range p.Register {
		kind[n] = "register"
	}
	for _, n := range p.Chosen {
		kind[n] = "chosen"
	}
	var b strings.Builder
	for _, f := range funcs {
		if p.Set[f.Name] {
			fmt.Fprintf(&b, "%s %s frame %d args %d height %d\n", f.Name, kind[f.Name], f.Frame, f.ArgBytes, p.Height[f.Name])
		}
	}
	return b.String()
}
