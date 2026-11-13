// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
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
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/whatwgurl"
)

// doorReads are the names through which Go reads a string a script gave, or
// makes one for it: only door.go may use them.
var doorReads = map[string]bool{
	"String": true, "WTF8": true, "Latin1": true, "UTF16": true, "Value": true, "This": true,
	"NewString": true, "NewStringLatin1": true, "NewStringUTF16": true, "NewInput": true, "StringLen": true,
	"Chars": true, "Open": true,
}

// doorLeaks returns what the files of package url, by name, do around the
// door: a use of a name of doorReads in a file other than door.go, and an
// import of a package that could parse a string without an Input.
func doorLeaks(files map[string]*ast.File) []string {
	var leaks []string
	for name, f := range files {
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, "github.com/liustack/quickjs/internal/whatwgurl/") || p == "net/url" {
				leaks = append(leaks, name+" imports "+p)
			}
		}
		if filepath.Base(name) == "door.go" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.SelectorExpr)
			if !ok || !doorReads[s.Sel.Name] {
				return true
			}
			// The type quickjs.Value is not a read.
			if id, ok := s.X.(*ast.Ident); ok && id.Name == "quickjs" && s.Sel.Name == "Value" {
				return true
			}
			leaks = append(leaks, name+" uses "+s.Sel.Name)
			return true
		})
	}
	sort.Strings(leaks)
	return leaks
}

// parseFiles parses the Go files of dir that are not tests.
func parseFiles(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = f
	}
	if len(files) == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	return files
}

// TestDoorIsTheOnlyWayIn reads the source of the package: a string of
// javascript comes into Go through admit only, and goes back through emit
// only, both in door.go. A new file that reads an argument, makes a string for
// javascript or makes an Input outside door.go fails it. And package
// whatwgurl takes no string from outside but in NewInput: every other
// exported function and method takes an Input.
func TestDoorIsTheOnlyWayIn(t *testing.T) {
	if leaks := doorLeaks(parseFiles(t, ".")); len(leaks) > 0 {
		t.Errorf("around the door:\n%s", strings.Join(leaks, "\n"))
	}
	for name, f := range parseFiles(t, "../internal/whatwgurl") {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Name.Name == "NewInput" {
				continue
			}
			if fn.Recv != nil {
				// A method of a type that is not exported is not reached from outside.
				recv := fn.Recv.List[0].Type
				if s, ok := recv.(*ast.StarExpr); ok {
					recv = s.X
				}
				if id, ok := recv.(*ast.Ident); ok && !id.IsExported() {
					continue
				}
			}
			for _, p := range fn.Type.Params.List {
				switch typ := p.Type.(type) {
				case *ast.Ident:
					if typ.Name == "string" {
						t.Errorf("%s: %s takes a string", name, fn.Name.Name)
					}
				case *ast.ArrayType:
					t.Errorf("%s: %s takes a slice", name, fn.Name.Name)
				}
			}
		}
	}
}

// TestDoorCheckCatches checks doorLeaks finds each way around the door.
func TestDoorCheckCatches(t *testing.T) {
	for _, body := range []string{
		`s, _ := a.String(0)`,
		`s, _ := a.WTF8(0)`,
		`b, _ := a.Latin1(0)`,
		`v := a.Value(0)`,
		`v := a.This()`,
		`s, _ := intlhook.String(a, 0)`,
		`in, _ := whatwgurl.NewInput("x")`,
		`v, _ := a.VM().NewString("x")`,
		`n, _ := a.StringLen(0)`,
		`b, u, release, _ := vmhook.Chars(a, 0)`,
		`d := intlhook.Open(a)`,
	} {
		src := "package url\nfunc f(a quickjs.Args) {\n" + body + "\n}\n"
		f, err := parser.ParseFile(token.NewFileSet(), "new.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if leaks := doorLeaks(map[string]*ast.File{"new.go": f}); len(leaks) != 1 {
			t.Errorf("%s: found %v", body, leaks)
		}
	}
	f, _ := parser.ParseFile(token.NewFileSet(), "new.go", "package url\nimport _ \"github.com/liustack/quickjs/internal/whatwgurl/idna\"\n", 0)
	if leaks := doorLeaks(map[string]*ast.File{"new.go": f}); len(leaks) != 1 {
		t.Errorf("an import of idna: found %v", leaks)
	}
}

// ropeJS defines rope(n): a string of n units, "http://h/" and then a, made by
// joining halves, so the engine holds it in pieces and it takes little memory
// whatever n is.
const ropeJS = `
	function ropeOf(n) {
		if (n <= 4096) {
			return "a".repeat(n);
		}
		const h = ropeOf(n >> 1);
		return n & 1 ? h + h + "a" : h + h;
	}
	function rope(n) { return "http://h/" + ropeOf(n - 9); }
	"ok"`

// TestStringLen checks Args.StringLen, as the door reads it, against the length javascript sees,
// for strings the engine holds in one piece, one byte or two a unit, and for
// ropes up to the longest string the engine makes, which it reads without
// joining them, and that it says false for what is not a string.
func TestStringLen(t *testing.T) {
	vm := newVM(t)
	b := intlhook.NewBuiltin("stringLen", 1, func(a quickjs.Args) (quickjs.Value, error) {
		n, ok := a.StringLen(0)
		if !ok {
			return a.VM().NewInt(-1), nil
		}
		return a.VM().NewFloat64(float64(n)), nil
	})
	fn, err := vm.NewBuiltinFunction(b)
	if err != nil {
		t.Fatal(err)
	}
	g := vm.GlobalObject()
	atom, _ := vm.NewAtom("stringLen")
	if err := vm.SetPropertyValue(g, atom, fn); err != nil {
		t.Fatal(err)
	}
	g.Free()
	evalString(t, vm, ropeJS)
	vm.RunGC()
	vm.SetMemoryLimit(vm.MemorySize() + 64<<20)
	defer vm.SetMemoryLimit(^uintptr(0))
	for js, want := range map[string]string{
		`stringLen("")`:                          "0",
		`stringLen("abc")`:                       "3",
		`stringLen("été")`:                       "3",
		`stringLen("一😀")`:                        "3",
		`stringLen("a".repeat(65537))`:           "65537",
		`stringLen(rope(65537))`:                 "65537",
		`stringLen(rope(1 << 20) + "一")`:         "1048577",
		`stringLen(rope((1 << 30) - 1))`:         "1073741823",
		`stringLen(1)`:                           "-1",
		`stringLen(undefined)`:                   "-1",
		`stringLen({toString() { return "x" }})`: "-1",
	} {
		if got := evalString(t, vm, "String("+js+")"); got != want {
			t.Errorf("%s = %s, want %s", js, got, want)
		}
	}
}

// doorEntries are calls each of which hands the string X to one Builtin, and
// what each answers when X is longer than the limit.
var doorEntries = []struct{ js, past string }{
	{`new URL(X).href.length`, "TypeError"},
	{`URL.canParse(X)`, "false"},
	{`URL.parse(X) === null`, "true"},
	{`new URL("/", X).href.length`, "TypeError"},
	{`var u = new URL("http://h/"); u.href = X; u.href.length`, "TypeError"},
	{`var u = new URL("http://h/"); u.pathname = X; u.href`, "http://h/"},
	{`var u = new URL("http://h/"); u.search = X; u.href`, "http://h/"},
	{`var u = new URL("http://h/"); u.hash = X; u.href`, "http://h/"},
	{`var u = new URL("http://h/"); u.username = X; u.href`, "http://h/"},
	{`var u = new URL("http://h/"); u.host = X; u.href`, "http://h/"},
}

// doorFormEntries hand X to formDecode or formEncode, after javascript, which
// the limits of the VM hold, made it flat.
var doorFormEntries = []struct{ js, past string }{
	{`new URLSearchParams("a=%" + X).get("a").length`, "RangeError"},
	{`new URLSearchParams([["a", X]]).toString().length`, "RangeError"},
	{`var u = new URL("http://h/?a=b"); try { u.searchParams.append("c", X) } catch (e) { e.name + u.href + u.searchParams.size }`, "RangeErrorhttp://h/?a=b1"},
}

// doorTries and doorTime: a refusal of a call takes doorTime or less in the
// fastest of doorTries calls of the same entry.
const (
	doorTries = 5
	doorTime  = 200 * time.Microsecond
)

// TestDoor checks that a string past the limit costs Go the same whatever its
// length: every Builtin refuses it by the length the engine keeps, before it
// reads a unit, so the time and the allocation of the call do not grow with
// it, up to a rope of 2^30-1 units, the longest string of the engine, which
// is not joined either. At the limit the call is made.
func TestDoor(t *testing.T) {
	const c = whatwgurl.MaxUnits
	type cost struct {
		alloc   int64
		took    time.Duration
		refused bool
	}
	run := func(vm *quickjs.VM, js string) (string, []cost) {
		var costs []cost
		var before runtime.MemStats
		var start time.Time
		admitted := false
		admitHook = func(int64) { admitted = true }
		callHook = func(enter bool) {
			if enter {
				admitted = false
				runtime.ReadMemStats(&before)
				start = time.Now()
				return
			}
			took := time.Since(start)
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			costs = append(costs, cost{int64(after.TotalAlloc - before.TotalAlloc), took, !admitted})
		}
		defer func() { callHook, admitHook = nil, nil }()
		got := evalString(t, vm, `(function () { try { return String(eval(`+strconv.Quote(js)+`)); } catch (e) { return e.name; } })()`)
		return got, costs
	}
	type length struct {
		name, js string
		forms    bool // whether the entries of formDecode and formEncode run
	}
	lengths := []length{
		{"MaxUnits+1", `"http://h/" + "a".repeat(` + strconv.Itoa(c+1-9) + `)`, true},
		{"16 x MaxUnits", `"http://h/" + "一".repeat(` + strconv.Itoa(16*c-9) + `)`, true},
		{"rope of 2^30-1", `rope((1 << 30) - 1)`, false},
	}
	vm := newVM(t)
	evalString(t, vm, ropeJS)
	for _, l := range lengths {
		evalString(t, vm, "var X = "+l.js+"; 'ok'")
		vm.RunGC()
		vm.SetMemoryLimit(vm.MemorySize() + 64<<20)
		all := doorEntries
		if l.forms {
			all = append(all[:len(all):len(all)], doorFormEntries...)
		}
		for _, e := range all {
			// The time of a refusal is the fastest of up to doorTries calls
			// of the same entry: a slow scheduler or the GC of another test
			// makes one call slow, not all of them, while work that grows
			// with the input makes every call slow. The allocation is held
			// on every call.
			best := time.Hour
			tries := doorTries
			if raceEnabled {
				tries = 1 // the time is not judged under the race detector
			}
			for try := 0; try < tries && best > doorTime; try++ {
				got, costs := run(vm, e.js)
				if got != e.past {
					t.Errorf("%s, %s = %s, want %s", l.name, e.js, got, e.past)
				}
				refused := 0
				for _, k := range costs {
					if !k.refused {
						continue
					}
					refused++
					// What the refusal costs: an error or a small number, no copy.
					if k.alloc > 1<<10 {
						t.Errorf("%s, %s: a call allocated %d bytes in Go", l.name, e.js, k.alloc)
					}
					best = min(best, k.took)
				}
				if refused != 1 {
					t.Errorf("%s, %s: %d calls refused X, want 1", l.name, e.js, refused)
				}
			}
			if best > doorTime && !raceEnabled {
				t.Errorf("%s, %s: the fastest of %d calls took %v, want %v or less", l.name, e.js, tries, best, doorTime)
			}
		}
		vm.SetMemoryLimit(^uintptr(0))
	}

	// At the limit, each is made.
	evalString(t, vm, `var X = "http://h/" + "a".repeat(`+strconv.Itoa(c-9)+`); 'ok'`)
	for js, want := range map[string]string{
		`new URL(X).href.length`: strconv.Itoa(c),
		`URL.canParse(X)`:        "true",
		`new URL("/", X).href`:   "http://h/",
		`var u = new URL("http://h/"); u.pathname = X.slice(8); u.href.length`:                       strconv.Itoa(c),
		`new URLSearchParams("a=%" + X.slice(9)).get("a").length`:                                    strconv.Itoa(c - 9 - 1), // %aa is one byte, not UTF-8: U+FFFD
		`new URLSearchParams([["a", "\u4e00".repeat(` + strconv.Itoa(c/9) + `)]]).toString().length`: strconv.Itoa(2 + c/9*9),
		`var u = new URL("http://h/?a=b"); u.searchParams.append("c", X.slice(16)); u.href.length`:   strconv.Itoa(c),
	} {
		if got, _ := run(vm, js); got != want {
			t.Errorf("at the limit, %s = %s, want %s", js, got, want)
		}
	}
}

// TestDoorNineLikeOne checks that admit allocates no more for nine strings
// than for one: it reads each length with Args.StringLen, which allocates
// nothing, and nothing in the door is made once a string.
func TestDoorNineLikeOne(t *testing.T) {
	vm := newVM(t)
	define := func(name string, idx ...int) {
		b := newBuiltin(name, len(idx), func(a quickjs.Args) (quickjs.Value, error) {
			admit(a, 0, 0, idx...)
			return quickjs.UndefinedValue, nil
		})
		f, err := vm.NewBuiltinFunction(b)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Free()
		set, err := vm.EvalValue(`(name, f) => { globalThis[name] = f }`, quickjs.EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		defer set.Free()
		if _, err := set.Call(quickjs.UndefinedValue, name, f); err != nil {
			t.Fatal(err)
		}
	}
	define("one", 0)
	define("nine", 0, 1, 2, 3, 4, 5, 6, 7, 8)
	const calls = 10000
	allocs := func(js string) float64 {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		evalString(t, vm, "for (var i = 0; i < "+strconv.Itoa(calls)+"; i++) { "+js+"; } 'ok'")
		runtime.ReadMemStats(&after)
		return float64(after.Mallocs-before.Mallocs) / calls
	}
	allocs(`one("")`)
	one, nine := allocs(`one("")`), allocs(`nine("", "", "", "", "", "", "", "", "")`)
	t.Logf("allocations a call: %.2f with one string, %.2f with nine", one, nine)
	if nine-one > 0.5 {
		t.Errorf("nine strings take %.2f allocations more than one", nine-one)
	}
}

// TestDoorSumsTheParts checks that admit refuses the parts of a record that
// sum to more than MaxUnits, which those of a URL never do, before it reads
// them, and counts in that sum only the parts: here the last two strings.
func TestDoorSumsTheParts(t *testing.T) {
	vm := newVM(t)
	b := newBuiltin("sum", 3, func(a quickjs.Args) (quickjs.Value, error) {
		_, _, v := admit(a, 0, 2, 0, 1, 2)
		return a.VM().NewInt(int(v)), nil
	})
	f, err := vm.NewBuiltinFunction(b)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Free()
	set, err := vm.EvalValue(`(f) => { globalThis.sum = f }`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer set.Free()
	if _, err := set.Call(quickjs.UndefinedValue, f); err != nil {
		t.Fatal(err)
	}
	half := strconv.Itoa(whatwgurl.MaxUnits / 2)
	for js, want := range map[string]verdict{
		`sum("", "a".repeat(` + half + `), "b".repeat(` + half + `))`:                           admitted,
		`sum("", "a".repeat(` + half + ` + 1), "b".repeat(` + half + `))`:                       notParts,
		`sum("v".repeat(2 * ` + half + `), "a".repeat(` + half + `), "b".repeat(` + half + `))`: admitted,
		`sum("", "a".repeat(2 * ` + half + ` + 1), "")`:                                         tooLong,
	} {
		v, err := vm.Eval(js, quickjs.EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := v.(int); verdict(got) != want {
			t.Errorf("%s: verdict %v, want %v", js, v, want)
		}
	}
}
