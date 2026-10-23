// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const registerChild = "QUICKJS_TEST_REGISTER_CHILD"

// TestRegisterExistingName registers Go functions under names the global
// object already has. Registering used to abort the process there, so the
// checks run in a child process: before the fix this test failed with the
// child's output instead of killing the whole test binary.
func TestRegisterExistingName(t *testing.T) {
	if os.Getenv(registerChild) == "" {
		var out lockedBuffer
		cmd := exec.Command(os.Args[0], "-test.run=^TestRegisterExistingName$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), registerChild+"=1")
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
		case <-time.After(2 * time.Minute):
			cmd.Process.Kill()
			// macOS keeps an aborted process alive, and its output pipe
			// open, until ReportCrash has written the crash report, which
			// can take far longer.
			t.Fatalf("the child process did not end in 2 minutes, it may have aborted\n%s", out.String())
		}
		return
	}

	testRegisterExistingName(t)
}

// lockedBuffer is a bytes.Buffer safe to read while a child process writes
// to it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// registeredMagics returns how many Go functions are registered in the
// process.
func registeredMagics() int {
	goFuncsMu.Lock()
	defer goFuncsMu.Unlock()
	return len(goFuncs)
}

// testRegisterExistingName is the body of TestRegisterExistingName, run in
// process by TestMemgrind2, so it must create and Close its own VMs.
func testRegisterExistingName(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	eval := func(js string) any {
		t.Helper()
		v, err := m.Eval(js, EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		return v
	}
	// replaced checks that name is now the Go function returning 2, with
	// the attributes a new Go function gets.
	replaced := func(name string) {
		t.Helper()
		js := `(() => {
			const d = Object.getOwnPropertyDescriptor(globalThis, ` + "'" + name + "'" + `);
			return [typeof d.value, d.value(), d.value.name, d.writable, d.configurable, d.enumerable].join();
		})()`
		if g, e := eval(js), "function,2,"+name+",true,true,false"; g != e {
			t.Errorf("%s: got %v, want %v", name, g, e)
		}
	}

	// A configurable property, as a script makes it by assignment, a
	// configurable accessor, and built-in globals: the Go function takes
	// their place.
	eval(`globalThis.g = 1`)
	eval(`Object.defineProperty(globalThis, 'acc', {get() { return 1 }, configurable: true})`)
	for _, name := range []string{"g", "acc", "Array", "JSON", "parseInt"} {
		if err := m.RegisterFunc(name, func() int { return 2 }, false); err != nil {
			t.Fatalf("RegisterFunc(%q): %v", name, err)
		}

		replaced(name)
	}
	if err := m.RegisterHostFunc("Math", func([]any) (any, error) { return 2, nil }); err != nil {
		t.Fatal(err)
	}

	replaced("Math")

	// A property that is not configurable stays as it is: the built-in
	// constants, and what a script declares with var or function.
	eval(`var v = 1; function f() { return 1 }`)
	for _, test := range []struct{ name, js string }{
		{"undefined", `typeof undefined`},
		{"NaN", `typeof NaN`},
		{"Infinity", `typeof Infinity`},
		{"v", `typeof v`},
		{"f", `typeof f() === 'number' ? 'kept' : 'lost'`},
	} {
		before := eval(test.js)
		n := registeredMagics()
		for _, host := range []bool{false, true} {
			if host {
				err = m.RegisterHostFunc(test.name, func([]any) (any, error) { return 2, nil })
			} else {
				err = m.RegisterFunc(test.name, func() int { return 2 }, false)
			}
			if err == nil || !strings.Contains(err.Error(), "not configurable") {
				t.Errorf("registering %q (host %v): got error %v, want one saying it is not configurable", test.name, host, err)
			}
		}
		if g := registeredMagics(); g != n {
			t.Errorf("registering %q failed and left %d Go functions registered, want %d", test.name, g, n)
		}
		if g := eval(test.js); g != before {
			t.Errorf("%s: %v after a failed register, want %v", test.js, g, before)
		}
	}

	// A name taken by a Go function, also once a script deleted it, and a
	// different Go string that becomes the same javascript name.
	alreadyRegistered := func(name string) {
		t.Helper()
		n := registeredMagics()
		if err := m.RegisterFunc(name, func() int { return 3 }, false); err == nil || !strings.Contains(err.Error(), "func already registered") {
			t.Errorf("RegisterFunc(%q) again: got error %v, want func already registered", name, err)
		}
		if g := registeredMagics(); g != n {
			t.Errorf("registering %q failed and left %d Go functions registered, want %d", name, g, n)
		}
	}
	alreadyRegistered("g")
	eval(`delete globalThis.g`)
	alreadyRegistered("g")
	if err := m.RegisterFunc("\xc0\x80", func() int { return 2 }, false); err != nil {
		t.Fatal(err)
	}

	alreadyRegistered("\xe4\xb8") // both are "��"
	if g, e := eval(`globalThis['��']()`), 2; g != e {
		t.Errorf("the first function under U+FFFD U+FFFD returns %v, want %v", g, e)
	}

	// A javascript name ends at a NUL byte, so a Go name with one would take
	// the place of another.
	if err := m.RegisterFunc("Array\x00b", func() int { return 2 }, false); err == nil {
		t.Errorf("RegisterFunc with a NUL byte in the name: no error")
	}

	// A name in brackets is a string like any other. The engine's function
	// lists read "[Symbol.iterator]" as that symbol, and aborted on any
	// other name in brackets.
	for _, name := range []string{"[foo]", "[Symbol.iterator]"} {
		if err := m.RegisterFunc(name, func() int { return 2 }, false); err != nil {
			t.Fatalf("RegisterFunc(%q): %v", name, err)
		}

		replaced(name)
	}
	if g, e := eval(`typeof globalThis[Symbol.iterator]`), "undefined"; g != e {
		t.Errorf("typeof globalThis[Symbol.iterator] = %v, want %v", g, e)
	}

	// A global object that takes no new properties.
	eval(`Object.preventExtensions(globalThis)`)
	n := registeredMagics()
	if err := m.RegisterFunc("fresh", func() int { return 2 }, false); err == nil {
		t.Errorf("RegisterFunc on a global object that is not extensible: no error")
	}
	if g := registeredMagics(); g != n {
		t.Errorf("registering on a global object that is not extensible left %d Go functions registered, want %d", g, n)
	}
	if g, e := eval(`typeof fresh`), "undefined"; g != e {
		t.Errorf("typeof fresh = %v, want %v", g, e)
	}
}
