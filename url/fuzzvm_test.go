// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package url

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/liustack/tifajs"
	"github.com/liustack/tifajs/internal/cengine"
	"github.com/liustack/tifajs/internal/whatwgurl"
)

// fuzzVMScript runs URL and URLSearchParams on I, against the base B when it
// is not empty, every way a script can: the constructor, URL.parse,
// URL.canParse, the getters, the setter K picks with the value V, and the
// methods of URLSearchParams. It checks what the package must keep, and
// gives the href of the URL, or null when I is not a URL.
const fuzzVMScript = `(() => {
	const fail = (what) => { const e = new Error(what); e.name = 'FuzzContract'; throw e }
	const setters = ['href', 'protocol', 'username', 'password', 'host', 'hostname', 'port', 'pathname', 'search', 'hash']
	let u = null
	try { u = B === '' ? new URL(I) : new URL(I, B) } catch (e) { if (!(e instanceof TypeError)) throw e }
	const p = B === '' ? URL.parse(I) : URL.parse(I, B)
	if ((p === null) !== (u === null)) fail('URL.parse and the constructor disagree')
	if ((B === '' ? URL.canParse(I) : URL.canParse(I, B)) !== (u !== null)) fail('URL.canParse and the constructor disagree')
	let href = null
	if (u) {
		href = u.href
		if (p.href !== href) fail('URL.parse gives ' + p.href)
		const parts = [u.origin, u.protocol, u.username, u.password, u.host, u.hostname, u.port, u.pathname, u.search, u.hash]
		if (new URL(href).href !== href) fail('the href does not parse to itself')
		if (u.toString() !== href || u.toJSON() !== href) fail('toString or toJSON is not the href')
		const sp = u.searchParams
		sp.append(V, V)
		sp.sort()
		if (u.search !== (sp.size ? '?' + sp.toString() : '')) fail('the search is not what searchParams writes')
		try { u[setters[K % setters.length]] = V } catch (e) { if (!(e instanceof TypeError)) throw e }
		parts.push(u.href, u.origin, u.searchParams.toString())
	}
	const q = new URLSearchParams(I)
	q.append(V, B)
	q.set(B, V)
	const keys = [...q.keys()]
	q.sort()
	const s = q.toString()
	if (new URLSearchParams(s).toString() !== s) fail('the text of URLSearchParams does not parse to itself')
	q.delete(V)
	keys.push(q.has(B), q.getAll(B).length, q.size, String(q))
	return href
})()`

// fuzzVMTimeout is the eval timeout of a VM of FuzzVM, and fuzzVMSlack how
// long past it a call may return.
const (
	fuzzVMTimeout  = 100 * time.Millisecond
	fuzzVMSlack    = 2 * time.Second
	fuzzVMWatchdog = 30 * time.Second
)

// FuzzVM runs fuzzVMScript in a VM with Register, an eval timeout and a
// memory limit that limit picks, and holds it to the contract of a call (see
// fuzz_test.go of package tifajs): no panic, no error but the exception of
// the script or an uncatchable end, no call past the timeout, and nothing
// left in the engine after Close. Where the input is valid UTF-8 the href is
// the one internal/whatwgurl parses.
func FuzzVM(f *testing.F) {
	for _, s := range [][2]string{
		{"http://example\t.\norg", "http://example.org/foo/bar"},
		{"http://user:pass@foo:21/bar;par?b#c", ""},
		{"/../x?a=1&b=%zz&c=é", "file:///C:/dir/"},
		{"https://xn--nxasmq6b.com/", ""},
		{"http://[::1]:80/", ""},
		{"http://0x7f.1/", ""},
		{"javascript:alert(1)", ""},
		{"#frag", "data:text/plain,hi"},
		{"a=1&a=2&b=%20&%F0%9F%98%80=x", ""},
		{"http://ß.com/\u0301", ""},
	} {
		f.Add(s[0], s[1], byte(0), "x", byte(0))
	}
	if fuzzingVM() {
		var data []json.RawMessage
		if b, err := os.ReadFile("../internal/whatwgurl/testdata/urltestdata.json"); err == nil && json.Unmarshal(b, &data) == nil {
			for i, r := range data {
				var c struct{ Input, Base *string }
				if json.Unmarshal(r, &c) != nil || c.Input == nil {
					continue
				}
				base := ""
				if c.Base != nil {
					base = *c.Base
				}
				f.Add(*c.Input, base, byte(i), "?a=b", byte(i>>2))
			}
		}
	}
	f.Fuzz(func(t *testing.T, input, base string, setter byte, value string, limit byte) {
		if len(input)+len(base)+len(value) > 4*whatwgurl.MaxUnits {
			return
		}
		leaked := cengine.Leaked()
		vm, err := tifajs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if err := Register(vm); err != nil {
			vm.Close()
			t.Fatal(err)
		}
		vm.SetEvalTimeout(fuzzVMTimeout)
		vm.SetMemoryLimit([...]uintptr{32 << 20, 4 << 20, 1 << 20}[limit%3])
		for _, g := range [][2]string{{"I", input}, {"B", base}, {"V", value}, {"K", string(rune('0' + setter%10))}} {
			if err := setGlobalErr(vm, g[0], g[1]); err != nil {
				var e *tifajs.Error
				vm.Close()
				if errors.As(err, &e) && e.OutOfMemory {
					return
				}
				t.Fatal(err)
			}
		}
		start := time.Now()
		dog := time.AfterFunc(fuzzVMWatchdog, func() {
			debug.SetTraceback("all")
			panic(fmt.Sprintf("fuzz: the URL script has not returned after %v", fuzzVMWatchdog))
		})
		got, err := vm.Eval(fuzzVMScript, tifajs.EvalGlobal)
		dog.Stop()
		if took := time.Since(start); took > fuzzVMTimeout+fuzzVMSlack {
			t.Fatalf("the URL script took %v with an eval timeout of %v (%v)", took, fuzzVMTimeout, err)
		}
		if err := vm.Close(); err != nil {
			t.Fatal(err)
		}
		if n := cengine.Leaked() - leaked; n != 0 {
			t.Fatalf("the runtime was freed with %d blocks still in use", n)
		}
		if n := keptStates(); n != 0 {
			t.Fatalf("the package keeps the state of %d VMs after Close", n)
		}
		var e *tifajs.Error
		switch {
		case err == nil:
		case !errors.As(err, &e):
			t.Fatalf("an error that is not an exception: %T %v", err, err)
		case e.Name == "FuzzContract":
			t.Fatalf("%q against %q: %s", input, base, e.Message)
		case e.Uncatchable:
			return
		case e.Name == "RangeError" && strings.Contains(e.Message, "longer than 65536"):
			return // a string past whatwgurl.MaxUnits
		default:
			t.Fatalf("%q against %q: the script threw %v", input, base, err)
		}

		// The href against what Go parses, for the inputs both read the
		// same way.
		if !utf8.ValidString(input) || !utf8.ValidString(base) {
			return
		}
		in, ok := whatwgurl.NewInput(input)
		if !ok {
			return
		}
		var b *whatwgurl.URL
		if base != "" {
			bi, ok := whatwgurl.NewInput(base)
			if !ok {
				return
			}
			if b, err = whatwgurl.Parse(bi, nil); err != nil {
				if got != nil {
					t.Fatalf("%q against %q gives %v, Go fails the base: %v", input, base, got, err)
				}
				return
			}
		}
		u, err := whatwgurl.Parse(in, b)
		switch {
		case err != nil && got != nil:
			t.Fatalf("%q against %q gives %v, Go fails it: %v", input, base, got, err)
		case err == nil && got == nil:
			t.Fatalf("%q against %q fails, Go gives %q", input, base, u.Href())
		case err == nil && got != u.Href():
			t.Fatalf("%q against %q gives %v, Go gives %q", input, base, got, u.Href())
		}
	})
}

// fuzzingVM reports whether the test runs with -fuzz.
func fuzzingVM() bool {
	f := flag.Lookup("test.fuzz")
	return f != nil && f.Value.String() != ""
}

// setGlobalErr sets the global name of vm to s.
func setGlobalErr(vm *tifajs.VM, name, s string) error {
	g := vm.GlobalObject()
	defer g.Free()
	a, err := vm.NewAtom(name)
	if err != nil {
		return err
	}
	return vm.SetProperty(g, a, s)
}
