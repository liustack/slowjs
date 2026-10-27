// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// noSplitCase is a small call graph for PickNoSplit: frames of 248 bytes
// take 256 with the saved base pointer.
func noSplitCase() ([]NoSplitFunc, map[string]bool) {
	u := func(name string, frame int, calls ...string) NoSplitFunc {
		return NoSplitFunc{Name: name, Kind: Unified, Frame: frame, ArgBytes: 8, Calls: calls}
	}
	fs := []NoSplitFunc{
		// a chain down to a leaf too big for the assembler to mark
		u("a", 32, "b"),
		u("b", 16, "c", "host_poll"),
		u("c", 200),
		// a leaf the assembler marks by itself, and one whose address is
		// taken, which it marks all the same
		u("tiny", 0),
		u("tinytaken", 64),
		// recursion, direct and through two functions, and their callers
		u("self", 16, "self"),
		u("p", 16, "q"),
		u("q", 16, "p", "c"),
		u("callsrec", 16, "self", "p"),
		// a function whose address is taken, one that calls through a
		// pointer, and their callers
		u("taken", 16, "c"),
		{Name: "icall", Kind: Unified, Frame: 16, ArgBytes: 8, Indirect: true},
		u("callsicall", 16, "icall", "taken"),
		// a chain too deep for the room: e2 does not fit, e1 does once e2
		// keeps its stack check
		u("e1", 248, "e2"),
		u("e2", 248, "e3"),
		u("e3", 248, "e4"),
		u("e4", 248),
		// the arguments of a Go func value's wrapper count above a function
		u("wide", 296, "e4"),
		u("narrow", 296, "e4"),
		// a register-entry leaf and its caller, a stub and its caller
		{Name: "memcpy", Kind: Register, Frame: 8},
		u("copies", 16, "memcpy"),
		{Name: "missing", Kind: Unified, ArgBytes: 16, Stub: true},
		u("callsstub", 16, "missing"),
	}
	for i := range fs {
		if fs[i].Name == "wide" {
			fs[i].ArgBytes = 200
		}
	}
	return fs, map[string]bool{"taken": true, "tinytaken": true}
}

var testCallbacks = map[string]bool{"host_poll": true}

// TestPickNoSplit: the choice follows the rules, counts the room as the
// linker does with a fault's room and a wrapper's on top, and says why it left
// each function out.
func TestPickNoSplit(t *testing.T) {
	fs, taken := noSplitCase()
	p, err := PickNoSplit(fs, taken, testCallbacks, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tiny", "tinytaken"}; !reflect.DeepEqual(p.Auto, want) {
		t.Errorf("auto leaves %v, want %v", p.Auto, want)
	}
	if want := []string{"memcpy"}; !reflect.DeepEqual(p.Register, want) {
		t.Errorf("register-entry leaves %v, want %v", p.Register, want)
	}
	if want := []string{"a", "b", "c", "callsicall", "callsrec", "callsstub", "copies", "e1", "e3", "e4", "narrow"}; !reflect.DeepEqual(p.Chosen, want) {
		t.Errorf("chosen %v, want %v", p.Chosen, want)
	}
	left := map[string][]string{
		"address taken":           {"taken"},
		"calls through a pointer": {"icall"},
		"on a cycle of calls":     {"p", "q", "self"},
		"over the room":           {"e2", "wide"},
		"stub":                    {"missing"},
	}
	for why, want := range left {
		got := append([]string{}, p.Left[why]...)
		sortStrings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("left for %q: %v, want %v", why, got, want)
		}
	}
	// local is the frame and 8 for the base pointer, 0 for a frameless
	// leaf; a leaf takes FaultRoom below its frame; a call takes 8, then the
	// callee's height, 8 for the morestack call of a function with a stack
	// check, or CallbackHeight for a Go callback.
	heights := map[string]int{
		"c":          208 + FaultRoom,
		"b":          max(24+8+208+FaultRoom, 24+8+CallbackHeight),
		"a":          40 + 8 + 24 + 8 + 208 + FaultRoom,
		"tiny":       FaultRoom,
		"tinytaken":  72 + FaultRoom,
		"e4":         256 + FaultRoom,
		"e3":         256 + 8 + 256 + FaultRoom,
		"e1":         256 + FaultRoom,
		"narrow":     304 + 8 + 256 + FaultRoom,
		"callsrec":   24 + FaultRoom,
		"callsicall": 24 + FaultRoom,
		"memcpy":     16 + FaultRoom,
		"copies":     24 + 8 + 16 + FaultRoom,
	}
	for n, want := range heights {
		if got := p.Height[n]; got != want {
			t.Errorf("height of %s: %d, want %d", n, got, want)
		}
	}
	// e2 needs 256+8+584 = 848, and wide would fit without its arguments.
	if h := p.Height["narrow"]; h+8+16 > NoSplitLimit || h+200+16 <= NoSplitLimit {
		t.Errorf("the case no longer tells wide from narrow: height %d", h)
	}
	for n := range p.Set {
		if p.Height[n] == 0 {
			t.Errorf("%s is in the set with no height", n)
		}
	}
	if r := NoSplitReport(p, fs); !strings.Contains(r, "a chosen frame 32 args 8 height ") || strings.Contains(r, "\nself ") {
		t.Errorf("the report:\n%s", r)
	}
}

// TestPickNoSplitOrder: the choice does not depend on the order of the
// functions or of their calls.
func TestPickNoSplitOrder(t *testing.T) {
	fs, taken := noSplitCase()
	want, err := PickNoSplit(fs, taken, testCallbacks, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(1))
	for range 20 {
		g := append([]NoSplitFunc{}, fs...)
		r.Shuffle(len(g), func(i, j int) { g[i], g[j] = g[j], g[i] })
		for i := range g {
			c := append([]string{}, g[i].Calls...)
			r.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
			g[i].Calls = c
		}
		got, err := PickNoSplit(g, taken, testCallbacks, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Set, want.Set) || !reflect.DeepEqual(got.Height, want.Height) {
			t.Fatalf("another order gives %v, want %v", got.Chosen, want.Chosen)
		}
	}
}

// TestPickNoSplitErrors: a call to a function the choice does not know, a
// name given twice and a forced name that is not generated are errors. A
// forced function goes in whatever the rules say, for the negative build.
func TestPickNoSplitErrors(t *testing.T) {
	fs, taken := noSplitCase()
	if _, err := PickNoSplit(append(fs, NoSplitFunc{Name: "x", Kind: Unified, Calls: []string{"nowhere"}}), taken, testCallbacks, nil); err == nil ||
		!strings.Contains(err.Error(), "nowhere") {
		t.Errorf("a call to an unknown function: %v", err)
	}
	if _, err := PickNoSplit(append(fs, fs[0]), taken, testCallbacks, nil); err == nil {
		t.Error("a function given twice was taken")
	}
	if _, err := PickNoSplit(fs, taken, testCallbacks, []string{"nowhere"}); err == nil {
		t.Error("a forced function that is not generated was taken")
	}
	p, err := PickNoSplit(fs, taken, testCallbacks, []string{"self", "p"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Set["self"] || !p.Set["p"] || p.Set["q"] {
		t.Errorf("forced: self %v p %v q %v", p.Set["self"], p.Set["p"], p.Set["q"])
	}
}

// TestPickNoSplitGeneratedFirst: a call goes to the generated function of
// that name even when the callbacks list it too (the driver's -allow list
// holds names the engine may define), so its height counts, not a
// callback's.
func TestPickNoSplitGeneratedFirst(t *testing.T) {
	fs := []NoSplitFunc{
		{Name: "malloc", Kind: Unified, Frame: 400, ArgBytes: 16, Calls: []string{"host_poll"}},
		{Name: "caller", Kind: Unified, Frame: 16, ArgBytes: 16, Calls: []string{"malloc"}},
	}
	p, err := PickNoSplit(fs, nil, map[string]bool{"host_poll": true, "malloc": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Set["malloc"] {
		t.Fatal("malloc was not chosen")
	}
	if want := 24 + 8 + p.Height["malloc"]; p.Height["caller"] != want {
		t.Errorf("height of caller %d, want %d", p.Height["caller"], want)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
