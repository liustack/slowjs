// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/liustack/slowjs"
	gointl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/intlhook"
)

// The work Intl does in Go for a script stays inside the limits of its VM:
// the memory limit and the eval timeout end the call, as they do in the
// engine, whatever the script asks of Intl.

// evalGuarded evaluates src on vm in a goroutine, and returns its result, its
// error, what it panicked with and how long it took. A call that has not
// returned by deadline fails the test.
func evalGuarded(t *testing.T, vm *slowjs.VM, src string, deadline time.Duration) (res any, err error, pan any, elapsed time.Duration) {
	t.Helper()
	type out struct {
		res any
		err error
		pan any
	}
	ch := make(chan out, 1)
	start := time.Now()
	go func() {
		var o out
		defer func() {
			o.pan = recover()
			ch <- o
		}()
		o.res, o.err = vm.Eval(src, slowjs.EvalGlobal)
	}()
	select {
	case o := <-ch:
		return o.res, o.err, o.pan, time.Since(start)
	case <-time.After(deadline):
		t.Fatalf("no answer after %v", deadline)
	}
	return
}

// wantUncatchable checks that err ended the call as running out of memory,
// or as an interrupt when interrupted is set.
func wantUncatchable(t *testing.T, err error, interrupted bool) {
	t.Helper()
	var e *slowjs.Error
	if !errors.As(err, &e) || !e.Uncatchable || e.Interrupted != interrupted || e.OutOfMemory == interrupted {
		t.Fatalf("got %v (%#v), want the uncatchable error of %s", err, e, map[bool]string{false: "running out of memory", true: "an interrupt"}[interrupted])
	}
}

// A long content argument that the VM has no memory to read ends the call
// with the out of memory error, in every Builtin: none takes it for the empty
// string and goes on, and none panics. The string is a rope of 32 MiB, which
// reading has to join, in a VM limited to 16 MiB. (A short argument that
// long is refused before it is read: TestShortArgumentRefused.)
func TestStringArgumentOutOfMemory(t *testing.T) {
	for _, js := range []string{
		`new Intl.Collator('en').compare(s, 'a')`,
		`s.localeCompare('a')`,
		`new Intl.Segmenter('en').segment(s).containing(0)`,
		`s.toLocaleUpperCase('tr')`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `new Intl.NumberFormat('en').format(1); 'x'.toLocaleUpperCase('tr'); JSON`)
			vm.SetMemoryLimit(16 << 20)
			_, err, pan, _ := evalGuarded(t, vm, `
				var r = "ok";
				try {
					let s = "1".repeat(1024);
					for (let i = 0; i < 15; i++) s = s + s;
					`+js+`;
				} catch (e) { r = "caught " + e; }
				r`, time.Minute)
			if pan != nil {
				t.Fatalf("panic: %v", pan)
			}
			wantUncatchable(t, err, false)
		})
	}
}

// A list a script passes Intl, however long, ends at the eval timeout: the
// work Go does over it checks the VM between batches. Each case takes Go
// several hundred milliseconds or more over a million tags or items, and the
// timeout is 20ms. The list comes from Go, made before the call, so that the
// time is all that of the Go work: none goes to the javascript of intl.js
// that makes the list a Builtin gets.
func TestLongListTimeout(t *testing.T) {
	tags := make([]string, 1<<20)
	var list strings.Builder
	for i := range tags {
		tags[i] = "zxx-x-" + strconv.FormatInt(int64(i), 36)
		list.WriteString(strconv.Itoa(len(tags[i])) + ":" + tags[i])
	}
	lf, err := resolveListFormat(request{Locales: []string{"en"}, Options: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		js string
		fn func(*slowjs.VM) (slowjs.Value, error)
	}{
		"getCanonicalLocales": {js: `bridge.getCanonicalLocales(undefined, bridge._list)`},
		"supportedLocalesOf":  {js: `bridge.supportedLocalesOf("NumberFormat", undefined, bridge._list)`},
		"resolve": {fn: func(vm *slowjs.VM) (slowjs.Value, error) {
			_, err := goResolve(vm, gointl.ServiceNumberFormat, tags)
			return slowjs.UndefinedValue, err
		}},
		"supported": {fn: func(vm *slowjs.VM) (slowjs.Value, error) {
			_, err := goSupported(vm, gointl.ServiceNumberFormat, tags)
			return slowjs.UndefinedValue, err
		}},
		"list": {fn: func(vm *slowjs.VM) (slowjs.Value, error) {
			items, _ := listItems(vm, list.String())
			return listParts(vm, items, lf.f.FormatToPartsCheck(items, func(i int) { checkTurn(vm, i) }))
		}},
		"stringArray": {fn: func(vm *slowjs.VM) (slowjs.Value, error) { return intlhook.EmitStrings(vm, tags) }},
	} {
		t.Run(name, func(t *testing.T) {
			vm := newVM(t)
			fns := map[string]*slowjs.Builtin{"work": intlhook.NewBuiltin("work", 0, func(a slowjs.Args) (slowjs.Value, error) {
				return c.fn(a.VM())
			})}
			for _, b := range bridge {
				f, err := vm.NewBuiltinFunction(b)
				if err != nil {
					t.Fatal(err)
				}
				n, err := vm.Call("(f => f.name)", f)
				if err != nil {
					t.Fatal(err)
				}
				fns[n.(string)] = b
				f.Free()
			}
			set, err := vm.EvalValue(`(name, f) => { (globalThis.bridge ??= {})[name] = f }`, slowjs.EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			defer set.Free()
			for n, b := range fns {
				f, err := vm.NewBuiltinFunction(b)
				if err != nil {
					t.Fatal(err)
				}
				_, err = set.Call(slowjs.UndefinedValue, n, f)
				f.Free()
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := set.Call(slowjs.UndefinedValue, "_list", list.String()); err != nil {
				t.Fatal(err)
			}
			js := c.js
			if js == "" {
				js = `bridge.work()`
			}
			if err := vm.SetEvalTimeout(20 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
			_, err, pan, elapsed := evalGuarded(t, vm, js, time.Minute)
			if pan != nil {
				t.Fatalf("panic: %v", pan)
			}
			wantUncatchable(t, err, true)
			if elapsed > time.Second {
				t.Fatalf("a timeout of 20ms ended the call after %v", elapsed)
			}
			t.Logf("ended after %v", elapsed)
		})
	}
}

// The work on a list asks Go for no memory sized by the whole list before
// its first check of the call: what it holds grows with the tags it has got
// through. The slices and the map goLocales made for every tag of a list of
// 1Mi came to 189 MiB, which Go cleared and the OS handed over before the
// first check: 30 to 103 ms on an AMD EPYC 7B13 and 4 to 27 ms on an Apple
// M4, against a timeout of 20 ms (TestLongListTimeout). Here the call is to
// stop before the work starts, so each runs to its first check, and a list
// of 1Mi tags may take no more Go memory there than one of 8Ki.
func TestLongListFirstCheck(t *testing.T) {
	tagsOf := func(n int) []string {
		tags := make([]string, n)
		for i := range tags {
			tags[i] = "zxx-x-" + strconv.FormatInt(int64(i), 36)
		}
		return tags
	}
	short, long := tagsOf(8<<10), tagsOf(1<<20)
	for name, fn := range map[string]func(vm *slowjs.VM, tags []string) error{
		"getCanonicalLocales": func(vm *slowjs.VM, tags []string) error {
			_, err := canonicalLocaleList(vm, tags)
			return err
		},
		"resolve": func(vm *slowjs.VM, tags []string) error {
			_, err := goResolve(vm, gointl.ServiceNumberFormat, tags)
			return err
		},
		"supported": func(vm *slowjs.VM, tags []string) error {
			_, err := goSupported(vm, gointl.ServiceNumberFormat, tags)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			vm := newVM(t)
			var tags []string
			b := intlhook.NewBuiltin("work", 0, func(a slowjs.Args) (slowjs.Value, error) {
				a.VM().Interrupt()
				return slowjs.UndefinedValue, fn(a.VM(), tags)
			})
			f, err := vm.NewBuiltinFunction(b)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Free()
			allocated := func(l []string) uint64 {
				tags = l
				var before, after runtime.MemStats
				runtime.ReadMemStats(&before)
				_, err := vm.Call("f => f()", f)
				runtime.ReadMemStats(&after)
				wantUncatchable(t, err, true)
				return after.TotalAlloc - before.TotalAlloc
			}
			allocated(short)
			few, many := allocated(short), allocated(long)
			if many > few+1<<20 {
				t.Errorf("Go allocated %d bytes up to the first check for 1Mi tags, %d for 8Ki", many, few)
			}
		})
	}
}

// A long tag resolves in time linear in its length: the matcher tries no
// part of it longer than the longest tag a service is available in. Cutting
// a private use tag of 64Ki subtags short one subtag at a time, trying each
// part, took two seconds. Such a tag is now refused at the door
// (TestShortArgumentRefused), and one at the cap of a tag resolves.
func TestLongTagResolves(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var tag = 'en-x-' + 'abcdefgh-'.repeat(1819) + 'x'; new Intl.NumberFormat('en'); 1`)
	for _, js := range []string{
		`new Intl.NumberFormat(tag).resolvedOptions().locale`,
		`Intl.Collator.supportedLocalesOf(tag)[0] === tag`,
	} {
		start := time.Now()
		got := eval(t, vm, js)
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("%s took %v", js, elapsed)
		}
		if got != "en" && got != true {
			t.Errorf("%s is %v", js, got)
		}
	}
}
