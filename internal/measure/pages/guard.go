// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package pages

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/liustack/beastjs"
	"github.com/liustack/beastjs/tifa"
)

// notHooks are the names __tifa_* a bundle may hold that tifa.Register does not define. ok says
// whether a bundle that holds one is measured: only __tifa_async_hooks__, which the bundle
// defines itself. The others belong to tifa's checks, and a bundle that asks for them was built
// for a check, not to be served.
var notHooks = map[string]struct {
	ok  bool
	why string
}{
	"__tifa_async_hooks__":   {true, "tifa's host script, inside the bundle, defines it for the app's code (js/host/host.ts of tifa), and asks the VM for nothing"},
	"__tifa_emit_check":      {false, "only a tifa program built with the tag tifa_emit_check defines it, for a bundle built with TIFA_EMIT_CHECK=1, which writes every tag twice to compare them: not a bundle to measure"},
	"__tifa_intl_warnings__": {false, "only tifa dev defines it (internal/ssr/intlcheck.js of tifa), and a bundle does not ask for it: build the bundle as tifa build does"},
	"__tifa_reads__":         {false, "only tifa dev defines it (internal/ssr/readcheck.js of tifa), and a bundle does not ask for it: build the bundle as tifa build does"},
}

// standInLeftOut are the hooks of tifa.Hooks the stand-in mode does not register, with why its
// numbers hold without them.
var standInLeftOut = map[string]string{
	"__tifa_random_bytes__":  "the program the stand-in mode reproduces did not register it. A load and a render of every page of engine/pgo/pages in the tifa mode call it 0 times (TestLeftOutNotCalled), and a bundle that called it without it would throw",
	"__tifa_async_context__": "the program the stand-in mode reproduces did not register it. A load and a render of every page of engine/pgo/pages in the tifa mode call its get and set 0 times (TestLeftOutNotCalled), and a bundle that called them without it would throw",
}

// kind is what a hook of tifa.Hooks is in a VM that has it: "get set" for the async context, an
// object with the functions get and set, "function" for the others.
func kind(name string) string {
	if name == "__tifa_async_context__" {
		return "get set"
	}
	return "function"
}

var hookName = regexp.MustCompile(`__tifa_[A-Za-z0-9_$]*`)

// Asked returns every name __tifa_* in the scripts, once each, sorted.
func Asked(scripts ...string) []string {
	var names []string
	for _, s := range scripts {
		for _, n := range hookName.FindAllString(s, -1) {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	slices.Sort(names)
	return names
}

// kindsOf is what a VM has under each name: "function", "get set" for an object with the
// functions get and set, or the typeof of anything else.
const kindsOf = `(names) => names.map((n) => {
  const v = globalThis[n]
  return typeof v === 'function' ? 'function' : v && typeof v.get === 'function' && typeof v.set === 'function' ? 'get set' : typeof v
}).join(',')`

// Have returns what vm has under each of names (see kindsOf). It compiles one small function in
// vm, which the collection after the bundle loads frees.
func Have(vm *beastjs.VM, names []string) (map[string]string, error) {
	have := map[string]string{}
	if len(names) == 0 {
		return have, nil
	}
	f, err := vm.EvalValue(kindsOf, beastjs.EvalGlobal)
	if err != nil {
		return nil, err
	}
	defer f.Free()
	r, err := f.Call(beastjs.UndefinedValue, names)
	if err != nil {
		return nil, err
	}
	s, ok := r.(string)
	if !ok {
		return nil, fmt.Errorf("pages: the kinds of the hooks are a %T", r)
	}
	for i, k := range strings.Split(s, ",") {
		have[names[i]] = k
	}
	return have, nil
}

// Guard checks each name __tifa_* the bundle asks for (asked) against what the VM of setup s has
// (have, from Have) and returns a line for each: registered, defined by the bundle, or left out
// with the reason. It fails, naming each, on a name it does not know, a hook s registers that
// the VM does not have as tifa gives it, and a name of tifa's checks.
func Guard(s Setup, asked []string, have map[string]string) ([]string, error) {
	var lines []string
	var errs []error
	for _, name := range asked {
		if n, ok := notHooks[name]; ok {
			if !n.ok {
				errs = append(errs, fmt.Errorf("the bundle asks for %s: %s", name, n.why))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s the bundle defines it: %s", name, n.why))
			continue
		}
		if !slices.Contains(tifa.Hooks(), name) {
			errs = append(errs, fmt.Errorf("the bundle asks for %s, which tifa.Register does not define and the guard does not know: add it to package tifa if tifa gives it to its VMs, or to the table of internal/measure/pages/guard.go with the reason", name))
			continue
		}
		switch {
		case s.Bare:
			lines = append(lines, name+" left out: -bare")
		case !slices.Contains(s.Registered(), name):
			why, ok := standInLeftOut[name]
			if s.Mode != StandIn || !ok {
				errs = append(errs, fmt.Errorf("the bundle asks for %s, which mode %s does not register and lists with no reason", name, s.Name()))
				continue
			}
			lines = append(lines, name+" left out: "+why)
		case have[name] != kind(name):
			errs = append(errs, fmt.Errorf("the bundle asks for %s, which the VM should have as a %s and has as %q", name, kind(name), have[name]))
		default:
			lines = append(lines, name+" registered")
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("pages: the guard stops the run (mode %s): %w", s.Name(), errors.Join(errs...))
	}
	return lines, nil
}
