// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command fixlib reapplies this fork's changes to the engine under lib/ after
// its files were taken from upstream libquickjs, as happens when a
// git subtree pull hits a conflict in a generated ccgo_*.go file and the
// upstream version is checked out. Run it from the repository root:
//
//	go run ./internal/fixlib         # apply, idempotent
//	go run ./internal/fixlib -check  # only report what is missing
//
// It does four things:
//
//   - In every lib/ccgo_*.go file, release the string reference that
//     XJS_ToCStringLen2 holds when allocating the UTF-8 copy fails. This is
//     the generated form of the fix in lib/internal/patch/quickjs.diff.
//   - In every lib/ccgo_*.go file, wire in the lazy error stacks of
//     lib/backtrace.go, the Go form of the backtrace change in the same
//     diff (see fixBacktrace).
//   - In the hand-written Go files under lib/, replace the import path
//     modernc.org/libquickjs with github.com/liustack/quickjs/lib.
//   - In every lib/internal/run-test262/ccgo_*.go file, make the engine API
//     functions of the test262 runner call the engine in lib/ (see
//     fixRunner), so that the runner tests that engine and not the copy ccgo
//     linked into it.
//
// It fails when a generated file no longer has the expected shape, which
// means upstream changed the code a fix touches and it needs a fresh look.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	oldPath = "modernc.org/libquickjs"
	newPath = "github.com/liustack/quickjs/lib"
)

var (
	check = flag.Bool("check", false, "report missing changes without writing files")

	// The two allocation failure branches of XJS_ToCStringLen2, with or
	// without the fix already in place.
	reAlloc = regexp.MustCompile(`(\t\tstr_new = _js_alloc_string\(tls, ctx2, [^\n]*, 0\)\n\t\tif !\(str_new != 0\) \{\n)(\t\t\tXFreeValue\(tls, ctx2, [^\n]*\)\n)?(\t\t\tgoto fail\n)`)
)

func main() {
	flag.Parse()
	missing, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixlib:", err)
		os.Exit(1)
	}
	if *check && missing != 0 {
		fmt.Fprintf(os.Stderr, "fixlib: %d file(s) need changes, run go run ./internal/fixlib\n", missing)
		os.Exit(1)
	}
}

func run() (missing int, err error) {
	generated, err := filepath.Glob(filepath.Join("lib", "ccgo_*.go"))
	if err != nil {
		return 0, err
	}
	if len(generated) == 0 {
		return 0, fmt.Errorf("no lib/ccgo_*.go files, run from the repository root")
	}
	for _, fn := range generated {
		changed, err := update(fn, func(b []byte) ([]byte, error) {
			b, err := fixToCString(b)
			if err != nil {
				return nil, err
			}
			return fixBacktrace(b)
		})
		if err != nil {
			return missing, err
		}
		if changed {
			missing++
		}
	}

	runners, err := filepath.Glob(filepath.Join("lib", "internal", "run-test262", "ccgo_*.go"))
	if err != nil {
		return missing, err
	}
	if len(runners) != len(generated) {
		return missing, fmt.Errorf("%d runner files for %d engine files", len(runners), len(generated))
	}
	for _, fn := range runners {
		engine, err := os.ReadFile(filepath.Join("lib", filepath.Base(fn)))
		if err != nil {
			return missing, err
		}
		// lib/test262_runner.go exports what ccgo left unexported.
		extra, err := os.ReadFile(filepath.Join("lib", "test262_runner.go"))
		if err != nil {
			return missing, err
		}
		engine = append(engine, extra...)
		changed, err := update(fn, func(b []byte) ([]byte, error) { return fixRunner(b, engine) })
		if err != nil {
			return missing, err
		}
		if changed {
			missing++
		}
	}

	err = filepath.WalkDir("lib", func(fn string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(fn, ".go") || strings.HasPrefix(d.Name(), "ccgo_") {
			return err
		}
		changed, err := update(fn, func(b []byte) ([]byte, error) {
			return bytes.ReplaceAll(b, []byte(`"`+oldPath+`"`), []byte(`"`+newPath+`"`)), nil
		})
		if changed {
			missing++
		}
		return err
	})
	return missing, err
}

// update rewrites fn with f, unless -check is set. It reports whether the
// content differs from what f returns.
func update(fn string, f func([]byte) ([]byte, error)) (changed bool, err error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return false, err
	}
	out, err := f(b)
	if err != nil {
		return false, fmt.Errorf("%s: %v", fn, err)
	}
	if bytes.Equal(b, out) {
		return false, nil
	}
	if *check {
		fmt.Println("needs changes:", fn)
		return true, nil
	}
	fmt.Println("updated:", fn)
	return true, os.WriteFile(fn, out, 0o644)
}

// fixToCString inserts the XFreeValue call into both allocation failure
// branches of XJS_ToCStringLen2.
func fixToCString(b []byte) ([]byte, error) {
	start := bytes.Index(b, []byte("\nfunc XJS_ToCStringLen2("))
	if start < 0 {
		return nil, fmt.Errorf("XJS_ToCStringLen2 not found")
	}
	end := bytes.Index(b[start:], []byte("\n}\n"))
	if end < 0 {
		return nil, fmt.Errorf("end of XJS_ToCStringLen2 not found")
	}
	end += start
	body := b[start:end]

	// ccgo keeps the C local 'val' in the TLS frame on 64-bit targets and in
	// a Go variable on the 32-bit ones.
	val := "val"
	if bytes.Contains(body, []byte("/* val at bp+32 */")) {
		val = "**(**TJSValue)(__ccgo_up(bp + 32))"
	} else if !bytes.Contains(body, []byte("\tvar val, ")) {
		return nil, fmt.Errorf("XJS_ToCStringLen2: cannot locate the local 'val'")
	}

	n := 0
	body = reAlloc.ReplaceAllFunc(body, func(m []byte) []byte {
		n++
		sub := reAlloc.FindSubmatch(m)
		return []byte(string(sub[1]) + "\t\t\tXFreeValue(tls, ctx2, " + val + ")\n" + string(sub[3]))
	})
	if n != 2 {
		return nil, fmt.Errorf("XJS_ToCStringLen2: want 2 allocation failure branches, found %d", n)
	}

	var out bytes.Buffer
	out.Write(b[:start])
	out.Write(body)
	out.Write(b[end:])
	return out.Bytes(), nil
}

var (
	reBuildBacktrace = regexp.MustCompile(`\nfunc _build_backtrace\(tls \*libc\.TLS, (\w+) uintptr, (\w+) TJSValue, filename uintptr, line_num int32, col_num int32, backtrace_flags int32\) \{\n`)
	reHasDebug       = regexp.MustCompile(`\n\t+if (int32\(\*\(\*uint8\)\(unsafe\.Pointer\(b \+ \d+\)\)&0x[0-9a-f]+>>\d+\)) != 0 \{\n\t+line_num1 = _find_line_num\(`)
	reGetPropString  = regexp.MustCompile(`\nfunc _get_prop_string\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, prop TJSAtom\) \(r uintptr\) \{\n`)
	rePropFlags      = regexp.MustCompile(`\n(\tif (int32\(\*\(\*uint32\)\(unsafe\.Pointer\(prs \+ 0\)\)&0x[0-9a-f]+>>\d+\))&\(libc\.Int32FromInt32\(3\)<<libc\.Int32FromInt32\(4\)\) != libc\.Int32FromInt32\(0\)<<libc\.Int32FromInt32\(4\) \{\n)`)
	rePropValue      = regexp.MustCompile(`\n\tval = \*\(\*TJSValue\)\(unsafe\.Pointer\((\*\*\(\*\*uintptr\)\(__ccgo_up\(bp( \+ \d+)?\)\))\)\)\n`)
	rePropObject     = regexp.MustCompile(`\n\tprs = _find_own_property\(tls, [^,]+, (\w+), prop\)\n`)
	reUncatchable    = regexp.MustCompile(`\nfunc XJS_SetUncatchableException\(tls \*libc\.TLS, ctx uintptr, flag TBOOL\) \{\n\tlibc\.SetBitFieldPtr16Int32\(\(\*TJSContext\)\(unsafe\.Pointer\(ctx\)\)\.Frt\+(\d+), flag, (\d+), (0x[0-9a-f]+)\)\n\}\n`)
	reRawDump        = regexp.MustCompile(`int32\(\*\(\*uint16\)\(unsafe\.Pointer\((s \+ \d+ \+ 0)\)\)&(0x[0-9a-f]+)>>(\d+)\)<<24>>24`)
	reMemoryHead     = regexp.MustCompile(`\nfunc XJS_ComputeMemoryUsage\(tls \*libc\.TLS, rt uintptr, s uintptr\) \{\n`)
	reMemoryProp     = regexp.MustCompile(`\n(\t+)if \(\*TJSShapeProperty\)\(unsafe\.Pointer\(prs\)\)\.Fatom != uint32\(MJS_ATOM_NULL\) && [^\n]*\{\n\t+_compute_value_size\(tls, \*\(\*TJSValue\)\(unsafe\.Pointer\(pr\)\), hp\)\n\t+\}\n`)

	// The read of a property value in JS_GetPropertyInternal and
	// JS_GetOwnPropertyInternal, where a pending stack must be formatted
	// first. The first argument of find_own_property is the room for the
	// JSProperty pointer, the second the object.
	reGetPropHead = regexp.MustCompile(`\nfunc XJS_GetPropertyInternal\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, prop TJSAtom, \w+ TJSValue, \w+ TBOOL\) \(r TJSValue\) \{\n`)
	reGetPropFind = regexp.MustCompile(`(\n[ \t]*prs = _find_own_property\(tls, (bp(?:\+\d+)?), (\w+), prop\)\n[ \t]*if prs != 0 \{\n)`)
	reGetOwnHead  = regexp.MustCompile(`\nfunc _JS_GetOwnPropertyInternal\(tls \*libc\.TLS, (\w+) uintptr, desc uintptr, (\w+) uintptr, prop TJSAtom\) \(r int32\) \{\n`)
	reGetOwnFind  = regexp.MustCompile(`(\n[ \t]*prs = _find_own_property\(tls, (bp(?:\+\d+)?), \w+, prop\)\n[ \t]*if prs != 0 \{\n)`)

	// The found-and-normal branch of the interpreter's get_field inlines,
	// which must take the slow path for a pending stack.
	reInterpField = regexp.MustCompile(`_find_own_property\(tls, (bp(?:\+\d+)?), \w+, (\w+)\)\n([ \t]+)if prs != 0 \{ /\* found \*/\n[ \t]+if [^\n]+\{\n[ \t]+goto (\w+_slow_path)\n[ \t]+\}\n`)

	// The two printer sites: js_print_error reads the stack, js_print_object
	// prints a normal property value.
	rePrintErrorHead  = regexp.MustCompile(`\nfunc _js_print_error\(tls \*libc\.TLS, s uintptr, p uintptr\) \{\n`)
	rePrintObjectHead = regexp.MustCompile(`\nfunc _js_print_object\(tls \*libc\.TLS, s uintptr, \w+ uintptr\) \{\n`)
	rePrintObjectVal  = regexp.MustCompile(`\n(\t+)_js_print_value\(tls, s, \*\(\*TJSValue\)\(unsafe\.Pointer\(pr\)\)\)\n`)
)

const (
	backtraceHelpers = "\n// Added by internal/fixlib for lib/backtrace.go.\n"
	memoryUsage      = "_js_backtrace_memory_usage(tls, s, hp, prs, pr)\n"
)

// fixBacktrace wires lib/backtrace.go into a generated file. 'stack' is an
// ordinary data property whose value, while pending, is a marker object, so
// the paths that read a property value must format the stack first:
//
//   - _build_backtrace leaves a stack without a file name to
//     _js_backtrace_build, which records it as a marker or formats it.
//   - JS_GetPropertyInternal and JS_GetOwnPropertyInternal call
//     _js_backtrace_settle on the stack property, which formats a pending
//     stack and stores the text.
//   - the interpreter's get_field inlines take the slow path, that is
//     JS_GetPropertyInternal, when the stack is pending.
//   - _get_prop_string calls _js_backtrace_prop_string, which formats a
//     pending stack for a print and never leaves an exception.
//   - js_print_error skips the stack of a raw dump, and js_print_object
//     prints a placeholder for a marker instead of the marker object.
//   - XJS_ComputeMemoryUsage counts the record of a pending stack.
//
// At the end it adds _js_backtrace_prop_flags, _js_backtrace_has_debug,
// _js_backtrace_uncatchable and _js_backtrace_raw_dump, with expressions
// copied from the generated code, as the bit fields they read differ
// between targets.
// ccgoUp renders an __ccgo_up deref of a find_own_property storage slot,
// which the generated code writes as "bp" or "bp+NN".
func ccgoUp(arg string) string {
	return "**(**uintptr)(__ccgo_up(" + strings.Replace(arg, "+", " + ", 1) + "))"
}

func fixBacktrace(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte(backtraceHelpers)); i >= 0 {
		b = b[:i] // the helpers are derived again below
	}

	body, err := funcBody(b, reBuildBacktrace, "_build_backtrace")
	if err != nil {
		return nil, err
	}
	m := reBuildBacktrace.FindSubmatch(b)
	ctx, errorObj := string(m[1]), string(m[2])
	hasDebug := reHasDebug.FindSubmatch(body)
	if hasDebug == nil {
		return nil, fmt.Errorf("_build_backtrace: cannot find the has_debug test")
	}
	lazy := fmt.Sprintf("\tif filename == 0 && _js_backtrace_build(tls, %s, %s, backtrace_flags) != 0 {\n\t\treturn\n\t}\n", ctx, errorObj)
	if !bytes.Contains(body, []byte(lazy)) {
		init := fmt.Sprintf("\t_js_dbuf_init(tls, %s, bp)\n", ctx)
		if bytes.Count(body, []byte(init)) != 1 {
			return nil, fmt.Errorf("_build_backtrace: want 1 call of _js_dbuf_init")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(init), []byte(lazy+init), 1), 1)
	}

	// get_prop_string: format a pending stack for a print.
	body, err = funcBody(b, reGetPropString, "_get_prop_string")
	if err != nil {
		return nil, err
	}
	gpsCtx := string(reGetPropString.FindSubmatch(b)[1])
	flags := rePropFlags.FindSubmatch(body)
	value := rePropValue.FindSubmatch(body)
	object := rePropObject.FindSubmatch(body)
	if flags == nil || value == nil || object == nil {
		return nil, fmt.Errorf("_get_prop_string: cannot find the property type test, the value or the object")
	}
	pending := fmt.Sprintf("\tif pending, ok := _js_backtrace_prop_string(tls, %s, %s, prs, %s); ok {\n\t\treturn pending\n\t}\n", gpsCtx, object[1], value[1])
	if !bytes.Contains(body, []byte(pending)) {
		b = bytes.Replace(b, body, bytes.Replace(body, flags[1], append([]byte(pending), flags[1]...), 1), 1)
	}

	// JS_GetPropertyInternal: settle the stack before the flag dispatch.
	if body, err = funcBody(b, reGetPropHead, "XJS_GetPropertyInternal"); err != nil {
		return nil, err
	}
	getCtx := string(reGetPropHead.FindSubmatch(b)[1])
	if !bytes.Contains(body, []byte("_js_backtrace_settle")) {
		fm := reGetPropFind.FindSubmatch(body)
		if fm == nil {
			return nil, fmt.Errorf("XJS_GetPropertyInternal: cannot find the property lookup")
		}
		ins := fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, %s) < 0 {\n\t\t\t\treturn jsvException()\n\t\t\t}\n\t\t}\n", getCtx, string(fm[3]), ccgoUp(string(fm[2])))
		b = bytes.Replace(b, body, bytes.Replace(body, fm[1], append(fm[1][:len(fm[1]):len(fm[1])], ins...), 1), 1)
	}

	// JS_GetOwnPropertyInternal: settle the stack before reading the value.
	if body, err = funcBody(b, reGetOwnHead, "_JS_GetOwnPropertyInternal"); err != nil {
		return nil, err
	}
	om := reGetOwnHead.FindSubmatch(b)
	ownCtx, ownObj := string(om[1]), string(om[2])
	if !bytes.Contains(body, []byte("_js_backtrace_settle")) {
		fm := reGetOwnFind.FindSubmatch(body)
		if fm == nil {
			return nil, fmt.Errorf("_JS_GetOwnPropertyInternal: cannot find the property lookup")
		}
		ins := fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) && desc != 0 {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, %s) < 0 {\n\t\t\t\treturn -int32(1)\n\t\t\t}\n\t\t}\n", ownCtx, ownObj, ccgoUp(string(fm[2])))
		b = bytes.Replace(b, body, bytes.Replace(body, fm[1], append(fm[1][:len(fm[1]):len(fm[1])], ins...), 1), 1)
	}

	// The interpreter's get_field inlines: bail to the slow path.
	if !bytes.Contains(b, []byte("_js_backtrace_pending")) {
		sites := reInterpField.FindAllSubmatchIndex(b, -1)
		if len(sites) == 0 {
			return nil, fmt.Errorf("interpreter: no get_field inline found")
		}
		var out bytes.Buffer
		last := 0
		for _, s := range sites {
			prBP := string(b[s[2]:s[3]])
			atom := string(b[s[4]:s[5]])
			indent := string(b[s[6]:s[7]])
			slow := string(b[s[8]:s[9]])
			out.Write(b[last:s[1]])
			fmt.Fprintf(&out, "%s\tif %s == uint32(EJS_ATOM_stack) && _js_backtrace_pending(tls, %s) {\n%s\t\tgoto %s\n%s\t}\n", indent, atom, ccgoUp(prBP), indent, slow, indent)
			last = s[1]
		}
		out.Write(b[last:])
		b = out.Bytes()
	}

	// js_print_error: skip the stack of a raw dump.
	if body, err = funcBody(b, rePrintErrorHead, "_js_print_error"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("_js_backtrace_raw_stack")) {
		end := bytes.Index(body, []byte(", uint32(EJS_ATOM_stack))"))
		if end < 0 {
			return nil, fmt.Errorf("_js_print_error: cannot find the stack read")
		}
		start := bytes.LastIndex(body[:end], []byte("str = _get_prop_string(tls,"))
		if start < 0 {
			return nil, fmt.Errorf("_js_print_error: cannot find the stack read start")
		}
		ls := bytes.LastIndexByte(body[:start], '\n') + 1
		indent := string(body[ls:start])
		guard := fmt.Sprintf("%sif _js_backtrace_raw_stack(tls, s, p) {\n%s\treturn\n%s}\n", indent, indent, indent)
		fixed := append(append(append([]byte{}, body[:ls]...), guard...), body[ls:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	// js_print_object: a placeholder for a marker value.
	if body, err = funcBody(b, rePrintObjectHead, "_js_print_object"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("_js_backtrace_print_pending")) {
		loc := rePrintObjectVal.FindSubmatchIndex(body)
		if loc == nil {
			return nil, fmt.Errorf("_js_print_object: cannot find the value print")
		}
		indent := string(body[loc[2]:loc[3]])
		call := string(body[loc[0]+1 : loc[1]-1]) // the _js_print_value(...) line, no surrounding newlines
		repl := fmt.Sprintf("\n%sif _js_backtrace_print_pending(tls, s, pr) == 0 {\n%s\t%s\n%s}\n", indent, indent, strings.TrimSpace(call), indent)
		fixed := append(append(append([]byte{}, body[:loc[0]]...), repl...), body[loc[1]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	// JS_ComputeMemoryUsage: count the record of a pending stack.
	if body, err = funcBody(b, reMemoryHead, "XJS_ComputeMemoryUsage"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte(memoryUsage)) {
		loc := reMemoryProp.FindAllSubmatchIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("XJS_ComputeMemoryUsage: want 1 size of a property value, found %d", len(loc))
		}
		indent := body[loc[0][2]:loc[0][3]]
		fixed := append(append(append(body[:loc[0][1]:loc[0][1]], indent...), memoryUsage...), body[loc[0][1]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	uncatchable := reUncatchable.FindSubmatch(b)
	if uncatchable == nil {
		return nil, fmt.Errorf("XJS_SetUncatchableException: cannot find the flag assignment")
	}
	rawDump := reRawDump.FindSubmatch(b)
	if rawDump == nil {
		return nil, fmt.Errorf("cannot find the raw_dump field")
	}

	var helpers bytes.Buffer
	fmt.Fprintf(&helpers, "package p\n%s", backtraceHelpers)
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_prop_flags(tls *libc.TLS, prs uintptr) int32 {\n\treturn %s\n}\n", flags[2])
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_uncatchable(tls *libc.TLS, ctx uintptr) TBOOL {\n\treturn TBOOL(*(*uint16)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Frt + %s)) & %s >> %s)\n}\n", uncatchable[1], uncatchable[3], uncatchable[2])
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_has_debug(tls *libc.TLS, b uintptr) int32 {\n\treturn %s\n}\n", hasDebug[1])
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_raw_dump(tls *libc.TLS, s uintptr) int32 {\n\treturn int32(*(*uint16)(unsafe.Pointer(%s))&%s>>%s) << 24 >> 24\n}\n", rawDump[1], rawDump[2], rawDump[3])
	formatted, err := format.Source(helpers.Bytes())
	if err != nil {
		return nil, fmt.Errorf("backtrace helpers: %v", err)
	}

	var out bytes.Buffer
	out.Write(bytes.TrimRight(b, "\n"))
	out.WriteString("\n")
	out.Write(bytes.TrimPrefix(formatted, []byte("package p\n")))
	return out.Bytes(), nil
}

// funcBody returns the text of the function whose head re matches.
func funcBody(b []byte, re *regexp.Regexp, name string) ([]byte, error) {
	loc := re.FindAllIndex(b, -1)
	if len(loc) != 1 {
		return nil, fmt.Errorf("want 1 function %s, found %d", name, len(loc))
	}
	start := loc[0][0]
	end := bytes.Index(b[start:], []byte("\n}\n"))
	if end < 0 {
		return nil, fmt.Errorf("end of %s not found", name)
	}
	return b[start : start+end+3], nil
}

var reFuncHead = regexp.MustCompile(`(?m)^func ([xX]\w+)\((.*?)\)( \(r [^()]+\))? \{$`)

const linkedImport = "\tlibquickjs \"github.com/liustack/quickjs/lib\"\n"

// fixRunner makes the test262 runner call the engine in lib/. ccgo linked a
// whole copy of the engine into the runner. Every exported engine function
// of that copy (x_JS_Eval, ...) whose counterpart in lib (XJS_Eval, ...) has
// the same parameters becomes a call of the counterpart, so the runner only
// reaches the copy through code that nothing calls. x_init, which sets up
// the runner's own globals, stays. The runtimes the runner creates get the
// stack limit of MaxStackSlots TLS slots that the runner's copy used.
func fixRunner(b, engine []byte) ([]byte, error) {
	exported := map[string]string{} // name -> parameters and result
	for _, m := range reFuncHead.FindAllSubmatch(engine, -1) {
		if name := string(m[1]); name[0] == 'X' {
			exported[name[1:]] = signature(string(m[2]), string(m[3]))
		}
	}

	var out bytes.Buffer
	last, n := 0, 0
	for _, loc := range reFuncHead.FindAllSubmatchIndex(b, -1) {
		name := string(b[loc[2]:loc[3]])
		if name[0] != 'x' || name == "x_init" {
			continue
		}
		params, result := string(b[loc[4]:loc[5]]), ""
		if loc[6] >= 0 {
			result = string(b[loc[6]:loc[7]])
		}
		sig, ok := exported[name[2:]]
		if !ok {
			continue
		}
		if sig != signature(params, result) {
			return nil, fmt.Errorf("%s: parameters differ from X%s", name, name[2:])
		}
		end := bytes.Index(b[loc[1]:], []byte("\n}\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of %s not found", name)
		}
		var args []string
		for _, p := range strings.Split(params, ", ") {
			args = append(args, strings.Fields(p)[0])
		}
		call := fmt.Sprintf("libquickjs.X%s(%s)", name[2:], strings.Join(args, ", "))
		var body string
		switch {
		case name == "x_JS_NewRuntime" || name == "x_JS_NewRuntime2":
			body = "\trt := " + call + "\n\tif rt != 0 {\n\t\tlibquickjs.XJS_SetMaxStackSize(tls, rt, libquickjs.MaxStackSlots)\n\t}\n\treturn rt"
		case result != "":
			body = "\treturn " + call
		default:
			body = "\t" + call
		}
		out.Write(b[last:loc[1]])
		out.WriteString("\n" + body)
		last = loc[1] + end
		n++
	}
	out.Write(b[last:])
	if n == 0 {
		return nil, fmt.Errorf("no engine function to forward")
	}

	b = out.Bytes()
	if !bytes.Contains(b, []byte(linkedImport)) {
		imp := []byte("\nimport (\n")
		if bytes.Count(b, imp) != 1 {
			return nil, fmt.Errorf("want 1 import block")
		}
		b = bytes.Replace(b, imp, append(imp[:len(imp):len(imp)], linkedImport...), 1)
	}
	return b, nil
}

// signature drops the parameter names of a function head, keeping the
// types.
func signature(params, result string) string {
	var types []string
	for _, p := range strings.Split(params, ", ") {
		if f := strings.Fields(p); len(f) == 2 {
			types = append(types, f[1])
		} else {
			types = append(types, p)
		}
	}
	return strings.Join(types, ", ") + "|" + strings.TrimPrefix(strings.TrimSpace(result), "(r ")
}
