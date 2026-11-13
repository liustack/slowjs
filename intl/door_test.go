// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The door of internal/intlhook is the only way a string of javascript comes
// into the Go of intl and temporal, and the only way one goes back.

// doorNames are the names through which Go reads a string a script gave, or
// makes one for it: only internal/intlhook may use them.
var doorNames = map[string]bool{
	"String": true, "WTF8": true, "Latin1": true, "UTF16": true, "StringLen": true, "Chars": true,
	"NewString": true, "NewStringLatin1": true, "NewStringUTF16": true, "NewArray": true, "Any": true,
}

// argsNames returns the names that hold a quickjs.Args in f: the parameters,
// fields and variables declared with that type.
func argsNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	isArgs := func(e ast.Expr) bool {
		s, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		id, ok := s.X.(*ast.Ident)
		return ok && id.Name == "quickjs" && s.Sel.Name == "Args"
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			if isArgs(n.Type) {
				for _, id := range n.Names {
					names[id.Name] = true
				}
			}
		case *ast.ValueSpec:
			if n.Type != nil && isArgs(n.Type) {
				for _, id := range n.Names {
					names[id.Name] = true
				}
			}
		}
		return true
	})
	return names
}

// doorLeaks returns what the Go files of dir that are not tests do around
// the door: a read of a string of an Args but its length check, a call of
// the reads of intlhook that the door wraps, and a string or an array made
// for javascript but by intlhook's Emit functions.
func doorLeaks(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var leaks []string
	files := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files++
		args := argsNames(f)
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.SelectorExpr)
			if !ok || !doorNames[s.Sel.Name] {
				return true
			}
			at := fset.Position(s.Pos()).String()
			switch x := s.X.(type) {
			case *ast.Ident:
				switch {
				case x.Name == "intlhook":
					leaks = append(leaks, at+": intlhook."+s.Sel.Name)
				case args[x.Name]:
					leaks = append(leaks, at+": "+x.Name+"."+s.Sel.Name+" of an Args")
				}
			case *ast.SelectorExpr:
				// c.a, the Args of a struct.
				if args[x.Sel.Name] {
					leaks = append(leaks, at+": ."+x.Sel.Name+"."+s.Sel.Name+" of an Args")
				}
			}
			switch s.Sel.Name {
			case "NewString", "NewStringLatin1", "NewStringUTF16", "NewArray", "Any":
				// Whatever it is called on: a VM makes them.
				if x, ok := s.X.(*ast.Ident); !ok || x.Name != "intlhook" {
					leaks = append(leaks, at+": "+s.Sel.Name)
				}
			}
			return true
		})
	}
	if files == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	sort.Strings(leaks)
	return leaks
}

// TestDoorIsTheOnlyWayIn reads the source of intl and temporal: a string of
// javascript comes into their Go through intlhook.Door only, and strings
// and arrays go back through intlhook's Emit functions only. A new read of
// an argument, or a new string made for javascript, outside the door fails
// it.
func TestDoorIsTheOnlyWayIn(t *testing.T) {
	for _, dir := range []string{".", "../temporal"} {
		if leaks := doorLeaks(t, dir); len(leaks) > 0 {
			t.Errorf("around the door, in %s:\n%s", dir, strings.Join(leaks, "\n"))
		}
	}
}

// The check of TestDoorIsTheOnlyWayIn sees each kind of leak.
func TestDoorLeaksSeen(t *testing.T) {
	dir := t.TempDir()
	src := `package x

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
)

type args struct{ a quickjs.Args }

func f(a quickjs.Args, c *args, vm *quickjs.VM) {
	a.String(0)
	a.WTF8(0)
	c.a.Latin1(0)
	intlhook.String(a, 0)
	vm.NewString("x")
	vm.NewArray()
	a.Value(0).Any()
	intlhook.Emit(vm, "x")
	a.Number(0)
}
`
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	leaks := doorLeaks(t, dir)
	if len(leaks) != 7 {
		t.Errorf("found %d leaks, want 7:\n%s", len(leaks), strings.Join(leaks, "\n"))
	}
}

// ropeJS makes s, a rope of 32 Mi units that is never joined: 1 Ki ones,
// doubled 15 times.
const ropeJS = `let s = "1".repeat(1024); for (let i = 0; i < 15; i++) s = s + s;`

// A short argument longer than the cap of its kind is refused with a
// RangeError before it is read: a rope of 32 Mi units is not joined, and
// Go allocates no more than a few KiB for the call.
func TestShortArgumentRefused(t *testing.T) {
	for _, js := range []string{
		`new Intl.NumberFormat('en').format(s)`,
		`new Intl.NumberFormat('en').formatToParts(s)`,
		`new Intl.NumberFormat(s)`,
		`Intl.getCanonicalLocales(s)`,
		`new Intl.Locale(s)`,
		`new Intl.DateTimeFormat('en', {calendar: s})`,
		`new Intl.NumberFormat('en', {numberingSystem: s})`,
		`Intl.supportedValuesOf(s)`,
		`dn.of(s)`,
		`rtf.format(1, s)`,
		`'x'.toLocaleUpperCase(s)`,
		`df.format(s)`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `new Intl.NumberFormat('en').format(1); 'x'.toLocaleUpperCase('tr'); var dn = new Intl.DisplayNames('en', {type: 'region'}), rtf = new Intl.RelativeTimeFormat('en'), df = new Intl.DurationFormat('en'); dn.of('FR'); rtf.format(1, 'day'); df.format({hours: 1}); 0`)
			eval(t, vm, `var s; { `+ropeJS+` globalThis.s = s; } 0`)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			got, err := vm.Eval(`(function () { try { `+js+`; return "made"; } catch (e) { return String(e); } })()`, quickjs.EvalGlobal)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			if g, _ := got.(string); !strings.HasPrefix(g, "RangeError") {
				t.Errorf("got %v, want a RangeError", got)
			}
			if n := after.TotalAlloc - before.TotalAlloc; n > 64<<10 {
				t.Errorf("Go allocated %d bytes for the call", n)
			}
		})
	}
}

// A short argument under its cap, in a VM with less room than the door
// counts for the call, ends the call with the out of memory error before it
// is read.
func TestShortArgumentNoRoom(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `new Intl.NumberFormat('en').format(1); var s = '1'.repeat(1000); 0`)
	runtime.GC()
	vm.SetMemoryLimit(vm.MemorySize() + 16<<10)
	_, err := vm.Eval(`try { new Intl.NumberFormat('en').format(s) } catch (e) { 'caught ' + e }`, quickjs.EvalGlobal)
	var e *quickjs.Error
	if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
		t.Fatalf("got %v, want the uncatchable out of memory error", err)
	}
}

// capsJS makes, for TestRoomCoversGo, the arguments of each kind at its
// cap that make Go take the most, and warms the data their calls read.
const capsJS = `
	const rep = (s, n) => s.repeat(Math.max(1, Math.ceil(n / s.length))).slice(0, n);
	const C = 16384, N = 524288;
	function variants(n) { let s = 'en'; for (let i = 0; s.length + 6 <= n; i++) s += '-a' + (1679616 + i).toString(36).slice(1); return s; }
	var tagVariants = variants(C), tagX = 'en-x-' + rep('abcdefgh-', C - 6) + 'a', tagT = 'en-t-' + rep('m0-abc-', C - 12) + 'm0-abc', tagU = 'en-u-' + rep('ca-gregory-', C - 16) + 'ca-abc';
	var typeSeq = rep('abcdefgh-', C - 1) + 'a', tz = rep('Abcdefgh/', C - 1) + 'A';
	var numFrac = '0.' + rep('1', N - 2), numZeros = '0.' + '0'.repeat(N - 3) + '1', numInt = rep('7', N);
	var fracFmt = new Intl.NumberFormat('en', {maximumFractionDigits: 100}), sig3 = new Intl.NumberFormat('en', {maximumSignificantDigits: 3});
	fracFmt.formatToParts(0.5); sig3.format(0.5);
	Intl.getCanonicalLocales('en-a0000-x-a-t-m0-abc-u-ca-gregory'); new Intl.NumberFormat('en-x-a').format(1); new Intl.DateTimeFormat('en-u-ca-abc').format(0);
	new Intl.Locale('en-u-ca-abc').getCalendars(); 'x'.toLocaleUpperCase('en-x-a'); new Intl.Collator('en', {collation: 'abc'}).compare('a', 'b');
	new Intl.DisplayNames('en', {type: 'calendar'}).of('abc'); new Intl.DisplayNames('en', {type: 'language'}).of('en-a0000');
	try { new Intl.DateTimeFormat('en', {timeZone: 'A/B'}) } catch (e) {}
	0`

// TestRoomCoversGo checks that what the door counts for the arguments of a
// call at the cap of their kind covers what Go takes for the call: with the
// room one byte short of what Go took, the door refuses it.
func TestRoomCoversGo(t *testing.T) {
	for _, js := range []string{
		`Intl.getCanonicalLocales(tagVariants)`,
		`Intl.getCanonicalLocales(tagX)`,
		`Intl.getCanonicalLocales(tagT)`,
		`Intl.getCanonicalLocales(tagU)`,
		`new Intl.NumberFormat(tagT).resolvedOptions().locale`,
		`new Intl.Locale(tagX.slice(0, C - 16)).maximize().toString()`,
		`new Intl.Locale(tagU).getCalendars()`,
		`new Intl.DateTimeFormat(tagU).resolvedOptions().locale`,
		`'x'.toLocaleUpperCase(tagX)`,
		`new Intl.DateTimeFormat('en', {calendar: typeSeq}).resolvedOptions().calendar`,
		`new Intl.NumberFormat('en', {numberingSystem: typeSeq}).resolvedOptions().numberingSystem`,
		`new Intl.Collator('en', {collation: typeSeq}).resolvedOptions().collation`,
		`new Intl.DisplayNames('en', {type: 'calendar'}).of(typeSeq)`,
		`new Intl.DisplayNames('en', {type: 'language'}).of(tagVariants)`,
		`new Intl.DateTimeFormat('en', {timeZone: tz})`,
		`fracFmt.formatToParts(numFrac)`,
		`sig3.format(numZeros)`,
		`sig3.format(numInt)`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, capsJS)
			src := `(function () { try { ` + js + ` } catch (e) { if (!(e instanceof RangeError)) throw e } return 0 })()`
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			eval(t, vm, src)
			runtime.ReadMemStats(&after)
			took := int64(after.TotalAlloc - before.TotalAlloc)

			room := vmhook.Room
			vmhook.Room = func(any) int64 { return took - 1 }
			defer func() { vmhook.Room = room }()
			_, err := vm.Eval(src, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.OutOfMemory {
				t.Errorf("Go took %d bytes, and with one less of room the call gave %v, want the out of memory error", took, err)
			}
		})
	}
}

// The arrays the Emit functions make leave nothing behind: the strings made
// for their elements are freed once the array holds them, so calls whose
// answers the script drops take no memory of the VM.
func TestEmittedArraysFreeTheirElements(t *testing.T) {
	for _, js := range []string{
		`Intl.getCanonicalLocales(['en-US', 'fr', 'de'])`, // EmitStrings
		`'İ\ud800x'.toLocaleLowerCase('tr')`,              // EmitMixed: a lone surrogate cuts the pieces
		`new Intl.Locale('en-US').getCalendars()`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			run := `for (let i = 0; i < 2000; i++) ` + js + `; 0`
			eval(t, vm, run)
			vm.RunGC()
			before := vm.MemoryUsage()
			eval(t, vm, run)
			vm.RunGC()
			after := vm.MemoryUsage()
			if d := after.MallocCount - before.MallocCount; d > 0 {
				t.Errorf("%d blocks more after 2,000 calls", d)
			}
		})
	}
}

// What the door counts for a string the case rules of a language map covers
// what Go takes for the call, at each of the rules' worst inputs: with the
// room one byte short of what Go took, the call ends with the out of memory
// error.
func TestCaseRoomCoversGo(t *testing.T) {
	const n = 1 << 20
	for _, js := range []string{
		`'i'.repeat(N).toLocaleUpperCase('tr')`,
		`'İ'.repeat(N).toLocaleLowerCase('tr')`,
		`('I' + '̣'.repeat(N - 1)).toLocaleLowerCase('tr')`,
		`'İ'.repeat(N / 2).toLocaleLowerCase('tr')`,
		`'Ì'.repeat(N).toLocaleLowerCase('lt')`,
		`'Į́'.repeat(N / 2).toLocaleLowerCase('lt')`,
		`'i̇'.repeat(N / 2).toLocaleUpperCase('lt')`,
		`'ᾷ'.repeat(N).toLocaleUpperCase('el')`,
		`'άι'.repeat(N / 2).toLocaleUpperCase('el')`,
		`'ή'.repeat(N).toLocaleUpperCase('el')`,
		`'\ud800i'.repeat(N / 2).toLocaleUpperCase('tr')`,
		`'\ud800ά'.repeat(N / 2).toLocaleUpperCase('el')`,
		`'𐐀i'.repeat(N / 3).toLocaleUpperCase('tr')`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			src := `(function () { var N = ` + strconv.Itoa(n) + `; ` + js + `; return 0 })()`
			eval(t, vm, src)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			eval(t, vm, src)
			runtime.ReadMemStats(&after)
			took := int64(after.TotalAlloc - before.TotalAlloc)
			t.Logf("Go took %.1f bytes a unit", float64(took)/n)

			room := vmhook.Room
			vmhook.Room = func(any) int64 { return took - 1 }
			defer func() { vmhook.Room = room }()
			_, err := vm.Eval(src, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.OutOfMemory {
				t.Errorf("Go took %d bytes, and with one less of room the call gave %v, want the out of memory error", took, err)
			}
		})
	}
}

// A compare of two strings of shortCompare units in all, which weighs them
// with no limits, takes less Go than the door's fixed room, for the
// characters of the most collation elements and the longest decompositions.
func TestShortCompareFitsFixedRoom(t *testing.T) {
	const half = shortCompare / 2
	for _, unit := range []string{`ﷺ`, `가`, `㍿`, `ǖ`, `ཱི`, `̖́`, `𝒜`} {
		t.Run(unit, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `var u = '`+unit+`', n = Math.floor(`+strconv.Itoa(half)+` / u.length), a = u.repeat(n), b = 'x' + u.repeat(n - 1), c = new Intl.Collator('en'); c.compare(a, b); 0`)
			if n := eval(t, vm, `a.length + b.length`); n.(int) > shortCompare {
				t.Fatalf("%d units", n)
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			eval(t, vm, `c.compare(a, b)`)
			runtime.ReadMemStats(&after)
			if took := after.TotalAlloc - before.TotalAlloc; took > intlhook.RoomFixed {
				t.Errorf("Go took %d bytes, more than the fixed room of %d", took, intlhook.RoomFixed)
			}
		})
	}
}
