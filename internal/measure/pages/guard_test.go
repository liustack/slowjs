// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/liustack/slowjs/tifa"
)

// all is what a VM of the tifa mode has: every hook of tifa.Hooks.
func all() map[string]string {
	have := map[string]string{}
	for _, h := range tifa.Hooks() {
		have[h] = "function"
	}
	have["__tifa_async_context__"] = "get set"
	return have
}

// Asked finds every name __tifa_* in the scripts, once each, sorted.
func TestAsked(t *testing.T) {
	got := Asked("var a = globalThis.__tifa_escape, b = __tifa_murmur2__('x')", "Reflect.deleteProperty(globalThis, `__tifa_escape`); __tifa_x$1")
	want := []string{"__tifa_escape", "__tifa_murmur2__", "__tifa_x$1"}
	if !slices.Equal(got, want) {
		t.Errorf("Asked = %q, want %q", got, want)
	}
}

// A hook the bundle asks for that the table does not know stops the run with an error that
// names it, in every mode: a new hook of a new tifa bundle must not be measured on React's
// fallback without anyone noticing.
func TestGuardUnknownHook(t *testing.T) {
	asked := []string{"__tifa_escape", "__tifa_new_hook__"}
	for _, s := range []Setup{{Mode: Tifa}, {Mode: StandIn}, {Mode: Tifa, Bare: true}} {
		_, err := Guard(s, asked, all())
		if err == nil || !strings.Contains(err.Error(), "__tifa_new_hook__") {
			t.Errorf("%s: error %v, want one naming __tifa_new_hook__", s.Name(), err)
		}
	}
}

// A hook the mode registers that the VM does not have as it should stops the run.
func TestGuardMissingHook(t *testing.T) {
	have := all()
	delete(have, "__tifa_start_tag")
	have["__tifa_async_context__"] = "function"
	_, err := Guard(Setup{Mode: Tifa}, []string{"__tifa_start_tag", "__tifa_async_context__", "__tifa_escape"}, have)
	if err == nil || !strings.Contains(err.Error(), "__tifa_start_tag") || !strings.Contains(err.Error(), "__tifa_async_context__") {
		t.Errorf("error %v, want one naming __tifa_start_tag and __tifa_async_context__", err)
	}
}

// A name only tifa's checks define (its emit check build, tifa dev) stops the run with the reason.
func TestGuardCheckHook(t *testing.T) {
	_, err := Guard(Setup{Mode: Tifa}, []string{"__tifa_emit_check"}, all())
	if err == nil || !strings.Contains(err.Error(), "TIFA_EMIT_CHECK") {
		t.Errorf("error %v, want the reason of __tifa_emit_check", err)
	}
}

// The stand-in mode leaves out the random bytes and the async context, with the reason, and the
// bare mode leaves out every hook and says so.
func TestGuardLeftOut(t *testing.T) {
	asked := []string{"__tifa_async_context__", "__tifa_async_hooks__", "__tifa_escape", "__tifa_random_bytes__"}
	have := all()
	delete(have, "__tifa_async_context__")
	delete(have, "__tifa_random_bytes__")
	r, err := Guard(Setup{Mode: StandIn}, asked, have)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(r, "\n")
	for _, w := range []string{"__tifa_escape registered", "__tifa_async_hooks__ the bundle defines it", "__tifa_random_bytes__ left out", "__tifa_async_context__ left out"} {
		if !strings.Contains(text, w) {
			t.Errorf("report has no %q:\n%s", w, text)
		}
	}
	r, err = Guard(Setup{Mode: Tifa, Bare: true}, asked, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	text = strings.Join(r, "\n")
	if !strings.Contains(text, "__tifa_escape left out: -bare") || strings.Contains(text, "registered") {
		t.Errorf("bare report:\n%s", text)
	}
}

// With the bundles of engine/pgo/pages, each hook a bundle asks for is registered in the VM of
// the tifa mode, or the bundle defines it itself, and in the stand-in and bare modes registered
// or left out with a reason: the guard passes on a VM set up for real.
func TestCheckedInBundles(t *testing.T) {
	repo, err := Repo("")
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range []string{"shop", "mui"} {
		src, err := os.ReadFile(filepath.Join(repo, "engine", "pgo", "pages", app, app+".js"))
		if err != nil {
			t.Fatal(err)
		}
		asked := Asked(string(src))
		if len(asked) < 7 {
			t.Fatalf("%s asks for %q only", app, asked)
		}
		for _, s := range []Setup{{Mode: Tifa}, {Mode: StandIn}, {Mode: Tifa, Bare: true}, {Mode: StandIn, Bare: true}} {
			vm, err := s.NewVM(defaultMemory, 0)
			if err != nil {
				t.Fatal(err)
			}
			have, err := Have(vm, asked)
			vm.Close()
			if err != nil {
				t.Fatal(err)
			}
			r, err := Guard(s, asked, have)
			if err != nil {
				t.Errorf("%s, %s: %v", app, s.Name(), err)
				continue
			}
			text := strings.Join(r, "\n")
			if s.Mode == Tifa && !s.Bare && strings.Contains(text, "left out") {
				t.Errorf("%s, tifa mode leaves a hook out:\n%s", app, text)
			}
			if s.Bare && strings.Contains(text, "registered") {
				t.Errorf("%s, %s registers a hook:\n%s", app, s.Name(), text)
			}
		}
	}
}

// Each mode has its name in the output and in the names of its Benchmark lines, so a file of
// results shows which one ran. The stand-in mode keeps the names of the program it reproduces.
func TestModeNames(t *testing.T) {
	for _, c := range []struct {
		s          Setup
		name, line string
	}{
		{Setup{Mode: Tifa}, "tifa", "tifa/"},
		{Setup{Mode: Tifa, Bare: true}, "tifa-bare", "tifa-bare/"},
		{Setup{Mode: StandIn}, "standin", ""},
		{Setup{Mode: StandIn, Bare: true}, "standin-bare", "standin-bare/"},
	} {
		if c.s.Name() != c.name || c.s.Prefix() != c.line {
			t.Errorf("%+v: name %q prefix %q, want %q %q", c.s, c.s.Name(), c.s.Prefix(), c.name, c.line)
		}
		if !strings.Contains(c.s.Describe(defaultMemory, 0), "mode "+c.name+":") {
			t.Errorf("%s: %q", c.name, c.s.Describe(defaultMemory, 0))
		}
	}
	if d := (Setup{Mode: Tifa, Bare: true}).Describe(defaultMemory, 0); !strings.Contains(d, "no tifa hook") {
		t.Errorf("the bare mode does not say so: %q", d)
	}
}

// Open stops at the guard with its error, and closes the VM it made, when a bundle asks for a
// name the guard does not know.
func TestOpenStopsAtTheGuard(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle.js")
	if err := os.WriteFile(bundle, []byte("globalThis.__tifa_new_thing__ && 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Setup{{Mode: Tifa}, {Mode: StandIn}} {
		r, err := Open(Options{Setup: s, Limit: defaultMemory, Timeout: defaultTimeout, Bundle: bundle})
		if r != nil || err == nil || !strings.Contains(err.Error(), "__tifa_new_thing__") {
			t.Errorf("%s: Open gave %v, %v, want the guard's error naming __tifa_new_thing__", s.Name(), r, err)
		}
	}
}
