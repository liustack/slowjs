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
	"sort"
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

	// The interpreter's inline property reads, the expansions of the C macro
	// GET_FIELD_INLINE (get_field, get_field2, get_length): a loop up the
	// prototype chain whose found branch takes the slow path for a property
	// that is not a plain value. After that test a guard sends a 'stack'
	// whose value is an object, as a pending stack is, to the slow path,
	// JS_GetPropertyInternal, which formats it.
	reInterpField = regexp.MustCompile(`\n[ \t]+for \{\n[ \t]+(prs\d*) = _find_own_property\(tls, ([^,\n]+), \w+, (\w+)\)\n[ \t]+if (prs\d*) != 0 \{ /\* found \*/\n([ \t]+)if [^\n]+\{\n[ \t]+goto (\w+_slow_path)\n[ \t]+\}\n`)
	reInterpGuard = regexp.MustCompile(`\n[ \t]+if \w+ == uint32\(EJS_ATOM_stack\) && _js_backtrace_object\([^\n]*\) \{\n[ \t]+goto \w+_slow_path\n[ \t]+\}`)
	// The interpreter's inline property write (put_field), which replaces a
	// plain value with set_value and so frees a marker like any value.
	reInterpPut    = regexp.MustCompile(`\n[ \t]+(prs\d*) = _find_own_property\(tls, [^,\n]+, \w+, \w+\)\n[ \t]+if !\((prs\d*) != 0\) \{\n[ \t]+goto put_field_slow_path\n`)
	reCallInternal = regexp.MustCompile(`\nfunc _JS_CallInternal\(tls \*libc\.TLS, [^\n]*\{\n`)

	// The SameValue test of JS_DefineProperty on a non-writable,
	// non-configurable data property, which must compare the text of a
	// pending stack.
	reDefineHead = regexp.MustCompile(`\nfunc XJS_DefineProperty\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, prop TJSAtom, \w+ TJSValue, \w+ TJSValue, \w+ TJSValue, flags int32\) \(r int32\) \{\n`)
	reDefineFind = regexp.MustCompile(`\n[ \t]+([^\n=]+?) = _find_own_property\(tls, [^,\n]+, (\w+), prop\)\n`)
	reDefineSame = regexp.MustCompile(`\n([ \t]+)if !\(_js_same_value\(tls, \w+, \w+, \*\(\*TJSValue\)\(unsafe\.Pointer\(([^\n]+?)\)\)\) != 0\) \{\n`)

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

// ccgoUp renders an __ccgo_up deref of a find_own_property storage slot,
// which the generated code writes as "bp" or "bp+NN".
func ccgoUp(arg string) string {
	return "**(**uintptr)(__ccgo_up(" + strings.Replace(arg, "+", " + ", 1) + "))"
}

// fixBacktrace wires lib/backtrace.go into a generated file. 'stack' is an
// ordinary data property whose value, while pending, is a marker object, so
// every path that hands a property value on must format the stack first:
//
//   - _build_backtrace leaves a stack without a file name to
//     _js_backtrace_build, which records it as a marker or formats it.
//   - JS_GetPropertyInternal and JS_GetOwnPropertyInternal call
//     _js_backtrace_settle on the stack property, which formats a pending
//     stack and stores the text. JS_DefineProperty does too before it
//     compares the value of a non-writable, non-configurable property.
//   - every inline property read of the interpreter (get_field, get_field2,
//     get_length) sends a 'stack' holding an object to the slow path,
//     JS_GetPropertyInternal.
//   - the callers of JS_GetOwnPropertyInternal that only look at the
//     attributes call _js_backtrace_own_flags, which leaves the stack
//     pending.
//   - _get_prop_string calls _js_backtrace_prop_string, which formats a
//     pending stack for a print and never leaves an exception.
//   - js_print_error skips the stack of a raw dump, and js_print_object
//     prints a placeholder for a marker instead of the marker object.
//   - XJS_ComputeMemoryUsage counts the record of a pending stack.
//
// auditSlots then checks that the functions that can reach a property value
// are the ones slotSites lists, so that new code upstream that reads the
// value of 'stack' in a new way cannot go unnoticed. At the end it adds
// _js_backtrace_prop_flags, _js_backtrace_has_debug,
// _js_backtrace_uncatchable and _js_backtrace_raw_dump, with expressions
// copied from the generated code, as the bit fields they read differ
// between targets.
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
		ins := fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, prs, %s) < 0 {\n\t\t\t\treturn jsvException()\n\t\t\t}\n\t\t}\n", getCtx, string(fm[3]), ccgoUp(string(fm[2])))
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
		ins := fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) && desc != 0 {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, prs, %s) < 0 {\n\t\t\t\treturn -int32(1)\n\t\t\t}\n\t\t}\n", ownCtx, ownObj, ccgoUp(string(fm[2])))
		b = bytes.Replace(b, body, bytes.Replace(body, fm[1], append(fm[1][:len(fm[1]):len(fm[1])], ins...), 1), 1)
	}

	// The interpreter: every inline read sends a pending 'stack' to the
	// slow path, and every _find_own_property call of JS_CallInternal is
	// either such a read or the put_field write, so that a new inline copy
	// fails here.
	if b, err = fixInterpreter(b); err != nil {
		return nil, err
	}

	// JS_DefineProperty: compare the text of a pending stack.
	if body, err = funcBody(b, reDefineHead, "XJS_DefineProperty"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("_js_backtrace_settle")) {
		defCtx := string(reDefineHead.FindSubmatch(b)[1])
		objs := reDefineFind.FindAllSubmatch(body, -1)
		if len(objs) == 0 {
			return nil, fmt.Errorf("XJS_DefineProperty: cannot find the property lookup")
		}
		for _, o := range objs[1:] {
			if !bytes.Equal(o[1], objs[0][1]) || !bytes.Equal(o[2], objs[0][2]) {
				return nil, fmt.Errorf("XJS_DefineProperty: lookups on different objects")
			}
		}
		same := reDefineSame.FindAllSubmatchIndex(body, -1)
		if len(same) != 1 {
			return nil, fmt.Errorf("XJS_DefineProperty: want 1 SameValue test of a data property, found %d", len(same))
		}
		indent := string(body[same[0][2]:same[0][3]])
		pr := string(body[same[0][4]:same[0][5]])
		ins := fmt.Sprintf("\n%sif prop == uint32(EJS_ATOM_stack) && _js_backtrace_settle(tls, %s, %s, %s, %s) < 0 {\n%s\treturn -int32(1)\n%s}", indent, defCtx, objs[0][2], objs[0][1], pr, indent, indent)
		fixed := append(append(append([]byte{}, body[:same[0][0]]...), ins...), body[same[0][0]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	// The callers of JS_GetOwnPropertyInternal that only look at the
	// attributes call _js_backtrace_own_flags, which does not format.
	for _, fn := range ownFlagsCallers {
		if b, err = fixOwnFlags(b, fn.name, fn.calls); err != nil {
			return nil, err
		}
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
		repl := fmt.Sprintf("\n%sif _js_backtrace_print_pending(tls, s, prs, pr) == 0 {\n%s\t%s\n%s}\n", indent, indent, strings.TrimSpace(call), indent)
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

	if err := auditSlots(b); err != nil {
		return nil, err
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

// slotSites lists every function of the generated engine that can reach the
// value of an own property, with how often it calls _find_own_property or
// _find_own_property1 (find), touches an object's property array (prop),
// calls JS_GetOwnPropertyInternal, JS_GetOwnProperty or
// _js_backtrace_own_flags, the definitions included (own), and whether it
// takes a property pointer named pr (pr). A pending stack is a marker object
// in the value of 'stack', so each of these must format it before handing
// the value on, or have a reason not to. fixlib checks every generated file
// against this list, so that a function upstream adds or changes, which may
// read the value of 'stack' in a new way, fails the check until someone has
// looked at it and updated the list.
var slotSites = map[string]slotSite{
	"_resize_properties":                       {0, 2, 0, 0, "grows the property array"},
	"_compact_properties":                      {0, 3, 0, 0, "moves values to a smaller array"},
	"_JS_NewObjectFromShape":                   {0, 5, 0, 0, "sets up the values of a new object"},
	"_js_autoinit_get_realm":                   {0, 0, 0, 1, "autoinit properties only"},
	"_js_autoinit_get_id":                      {0, 0, 0, 1, "autoinit properties only"},
	"_js_autoinit_free":                        {0, 0, 0, 1, "autoinit properties only"},
	"_js_autoinit_mark":                        {0, 0, 0, 1, "autoinit properties only"},
	"_free_property":                           {0, 0, 0, 1, "frees a value: the marker's class frees the record"},
	"_find_own_property1":                      {1, 0, 0, 0, "the lookup itself"},
	"_find_own_property":                       {1, 1, 0, 0, "the lookup itself"},
	"_free_object":                             {0, 3, 0, 0, "frees the values"},
	"_mark_children":                           {0, 1, 0, 0, "marks the values: the marker's class marks the bytecode"},
	"XJS_ComputeMemoryUsage":                   {0, 2, 0, 0, "sizes the values: hooked, counts the record"},
	"_get_prop_string":                         {2, 0, 0, 0, "native read for a print: hooked, formats without leaving an exception"},
	"_is_backtrace_needed":                     {1, 0, 0, 0, "whether 'stack' exists"},
	"_JS_AutoInitProperty":                     {0, 0, 0, 1, "autoinit properties only"},
	"XJS_GetPropertyInternal":                  {1, 0, 0, 0, "[[Get]]: hooked, formats first"},
	"_JS_DefinePrivateField":                   {1, 0, 0, 0, "private fields, never 'stack'"},
	"_JS_GetPrivateField":                      {1, 0, 0, 0, "private fields, never 'stack'"},
	"_JS_SetPrivateField":                      {1, 0, 0, 0, "private fields, never 'stack'"},
	"_JS_AddBrand":                             {2, 0, 0, 0, "private brands, never 'stack'"},
	"_JS_CheckBrand":                           {2, 0, 0, 0, "private brands, never 'stack'"},
	"_JS_GetOwnPropertyNamesInternal":          {0, 1, 1, 0, "names and enumerability: attributes only"},
	"_JS_GetOwnPropertyInternal":               {1, 0, 1, 0, "descriptor: hooked, formats first"},
	"XJS_GetOwnProperty":                       {0, 0, 2, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"XJS_HasProperty":                          {0, 0, 1, 0, "existence only"},
	"_add_property":                            {0, 4, 0, 0, "adds a property"},
	"_remove_global_object_property":           {0, 0, 0, 1, "global variables only"},
	"_delete_property":                         {0, 1, 0, 0, "frees the value"},
	"_set_array_length":                        {0, 3, 0, 0, "array length only"},
	"_add_fast_array_element":                  {0, 3, 0, 0, "array length only"},
	"_js_allocate_fast_array":                  {0, 1, 0, 0, "array length only"},
	"_js_create_array":                         {0, 1, 0, 0, "array length only"},
	"_js_create_array_free":                    {0, 1, 0, 0, "array length only"},
	"XJS_SetPropertyInternal":                  {2, 0, 1, 0, "replaces the value with set_value: attributes only"},
	"_JS_CreateProperty":                       {1, 1, 0, 0, "adds a property, array length only"},
	"XJS_DefineProperty":                       {2, 0, 0, 0, "replaces the value: hooked, formats before SameValue"},
	"_JS_DefineAutoInitProperty":               {1, 0, 0, 0, "adds an autoinit property"},
	"_js_object_has_name":                      {1, 0, 0, 0, "'name' only"},
	"_JS_CheckDefineGlobalVar":                 {2, 0, 0, 0, "global variables only"},
	"_JS_GetGlobalVarRef":                      {1, 0, 0, 0, "global variables only"},
	"_JS_DeleteGlobalVar":                      {1, 0, 0, 0, "global variables only"},
	"_udiv1norm":                               {0, 0, 0, 1, "big number code, pr is not a property"},
	"_js_print_array_get_length":               {1, 0, 0, 0, "'length' only"},
	"_js_print_object":                         {0, 1, 0, 0, "prints the values: hooked, a placeholder for the marker"},
	"_js_operator_private_in":                  {1, 0, 0, 0, "private brands, never 'stack'"},
	"_js_for_in_next":                          {0, 0, 2, 0, "existence only"},
	"_JS_CopyDataProperties":                   {0, 0, 2, 0, "attributes only, values read by JS_GetProperty"},
	"_js_global_object_get_uninitialized_var":  {1, 0, 0, 0, "global variables only"},
	"_js_global_object_find_uninitialized_var": {1, 0, 0, 0, "global variables only"},
	"_js_closure_define_global_var":            {3, 0, 0, 0, "global variables only"},
	"_js_closure_global_var":                   {2, 0, 0, 0, "global variables only"},
	"_JS_CallInternal":                         {4, 3, 0, 0, "inline reads hooked (see fixInterpreter), put_field replaces with set_value, array length"},
	"_seal_template_obj":                       {1, 0, 0, 0, "'length' attributes only"},
	"_js_module_ns_has":                        {1, 0, 0, 0, "module namespaces only"},
	"_JS_WriteArray":                           {2, 0, 0, 0, "arrays and template objects only"},
	"_JS_WriteObjectTag":                       {0, 1, 0, 0, "plain objects only, an Error is never written"},
	"_js_object_getOwnPropertyDescriptor":      {0, 0, 1, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"_JS_GetOwnPropertyNames2":                 {0, 0, 1, 0, "attributes only, values read by JS_GetProperty"},
	"_js_object_hasOwnProperty":                {0, 0, 1, 0, "existence only"},
	"_js_object_hasOwn":                        {0, 0, 1, 0, "existence only"},
	"_js_object_seal":                          {0, 0, 1, 0, "attributes only"},
	"_js_object_isSealed":                      {0, 0, 1, 0, "attributes only"},
	"_js_object_propertyIsEnumerable":          {0, 0, 1, 0, "attributes only"},
	"_js_object___lookupGetter__":              {0, 0, 1, 0, "attributes and accessors only"},
	"_js_function_bind":                        {0, 0, 1, 0, "existence of 'length' only"},
	"_js_array_push":                           {0, 3, 0, 0, "array length only"},
	"_js_iterator_proto_set_toStringTag":       {0, 0, 1, 0, "existence only"},
	"_js_regexp_get_lastIndex":                 {0, 3, 0, 0, "RegExp lastIndex only"},
	"_js_regexp_set_lastIndex":                 {0, 2, 0, 0, "RegExp lastIndex only"},
	"_find_property_regexp":                    {1, 0, 0, 0, "RegExp properties only"},
	"_js_is_standard_regexp":                   {1, 0, 0, 0, "RegExp properties only"},
	"_json_parse_record_init_obj":              {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_parse_record_init_array":            {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_parse_record_init_primitive":        {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_parse_record_add":                   {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_parse_record_find":                  {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_free_parse_record":                  {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_json_parse_value":                        {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"XJS_ParseJSON3":                           {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_internalize_json_property":               {0, 0, 0, 1, "JSON parse records, pr is not a property"},
	"_js_proxy_has":                            {0, 0, 1, 0, "attributes only"},
	"_js_proxy_get":                            {0, 0, 1, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"_js_proxy_set":                            {0, 0, 1, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"_js_proxy_get_own_property":               {0, 0, 2, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"_js_proxy_define_own_property":            {0, 0, 1, 0, "descriptor through JS_GetOwnPropertyInternal"},
	"_js_proxy_delete_property":                {0, 0, 1, 0, "attributes only"},
	"_js_proxy_get_own_property_names":         {0, 0, 1, 0, "attributes only"},
	"_udiv1norm1":                              {0, 0, 0, 1, "big number code, pr is not a property"},
}

type slotSite struct {
	find, prop, own, pr int
	why                 string
}

var (
	reSlotFind = regexp.MustCompile(`\b_find_own_property1?\(`)
	reSlotProp = regexp.MustCompile(`\.Fprop\b`)
	reSlotOwn  = regexp.MustCompile(`\b(?:_JS_GetOwnPropertyInternal|XJS_GetOwnProperty|_js_backtrace_own_flags)\(`)
	reSlotPr   = regexp.MustCompile(`^func \w+\([^)]*\bpr\d* uintptr`)
	reFuncName = regexp.MustCompile(`^func (\w+)`)
)

// auditSlots checks b against slotSites.
func auditSlots(b []byte) error {
	seen := map[string]bool{}
	var problems []string
	for _, fn := range bytes.Split(b, []byte("\nfunc "))[1:] {
		fn = append([]byte("func "), fn...)
		m := reFuncName.FindSubmatch(fn)
		if m == nil {
			continue
		}
		name := string(m[1])
		if strings.HasPrefix(name, "_js_backtrace_") {
			continue // the helpers fixlib adds
		}
		got := slotSite{find: len(reSlotFind.FindAll(fn, -1)), prop: len(reSlotProp.FindAll(fn, -1)), own: len(reSlotOwn.FindAll(fn, -1))}
		if reSlotPr.Match(fn) {
			got.pr = 1
		}
		want, ok := slotSites[name]
		if !ok {
			if got != (slotSite{}) {
				problems = append(problems, fmt.Sprintf("%s reaches property values and is not listed", name))
			}
			continue
		}
		seen[name] = true
		want.why = ""
		if got != want {
			problems = append(problems, fmt.Sprintf("%s: find, prop, own, pr are %d, %d, %d, %d, the list has %d, %d, %d, %d", name, got.find, got.prop, got.own, got.pr, want.find, want.prop, want.own, want.pr))
		}
	}
	for name := range slotSites {
		if !seen[name] {
			problems = append(problems, fmt.Sprintf("%s is listed but not found", name))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("property value access changed, review each for the pending stack and update slotSites:\n\t%s", strings.Join(problems, "\n\t"))
	}
	return nil
}

// fixInterpreter puts a guard in each inline property read of
// JS_CallInternal that sends a 'stack' holding an object to the slow path.
// Guards already there are dropped first, so the result does not depend on
// the input having them.
func fixInterpreter(b []byte) ([]byte, error) {
	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	fixed := reInterpGuard.ReplaceAll(body, nil)
	finds := bytes.Count(fixed, []byte("_find_own_property("))
	puts := len(reInterpPut.FindAll(fixed, -1))
	sites := reInterpField.FindAllSubmatchIndex(fixed, -1)
	if len(sites) == 0 {
		return nil, fmt.Errorf("_JS_CallInternal: no inline property read found")
	}
	if len(sites)+puts != finds {
		return nil, fmt.Errorf("_JS_CallInternal: %d property lookups, but %d inline reads and %d inline writes: review the new one for the pending stack", finds, len(sites), puts)
	}
	var out bytes.Buffer
	last := 0
	for _, s := range sites {
		if !bytes.Equal(fixed[s[2]:s[3]], fixed[s[8]:s[9]]) {
			return nil, fmt.Errorf("_JS_CallInternal: inline read tests another variable")
		}
		pr := ccgoUp(string(fixed[s[4]:s[5]]))
		atom := string(fixed[s[6]:s[7]])
		indent := string(fixed[s[10]:s[11]])
		slow := string(fixed[s[12]:s[13]])
		out.Write(fixed[last:s[1]])
		fmt.Fprintf(&out, "%sif %s == uint32(EJS_ATOM_stack) && _js_backtrace_object(%s) {\n%s\tgoto %s\n%s}\n", indent, atom, pr, indent, slow, indent)
		last = s[1]
	}
	out.Write(fixed[last:])
	return bytes.Replace(b, body, out.Bytes(), 1), nil
}

// ownFlagsCallers are the functions that call JS_GetOwnPropertyInternal
// with a descriptor only to read its attributes (or a getter or setter),
// never its value, and how many calls each has. They call
// _js_backtrace_own_flags instead, so that they do not format a pending
// stack.
var ownFlagsCallers = []struct {
	name  string
	calls int
}{
	{"_JS_GetOwnPropertyNamesInternal", 1},  // enumerability of exotic keys
	{"XJS_SetPropertyInternal", 1},          // writable before a redefine
	{"_JS_CopyDataProperties", 2},           // enumerability, values read by JS_GetProperty
	{"_JS_GetOwnPropertyNames2", 1},         // enumerability, values read by JS_GetProperty
	{"_js_object_seal", 1},                  // writable, for freeze
	{"_js_object_isSealed", 1},              // configurable and writable
	{"_js_object_propertyIsEnumerable", 1},  // enumerable
	{"_js_object___lookupGetter__", 1},      // getter or setter
	{"_js_proxy_has", 1},                    // configurable
	{"_js_proxy_delete_property", 1},        // configurable
	{"_js_proxy_get_own_property_names", 1}, // configurable
}

func fixOwnFlags(b []byte, name string, calls int) ([]byte, error) {
	re := regexp.MustCompile(`\nfunc ` + regexp.QuoteMeta(name) + `\(tls \*libc\.TLS, [^\n]*\{\n`)
	body, err := funcBody(b, re, name)
	if err != nil {
		return nil, err
	}
	fixed := bytes.ReplaceAll(body, []byte("_JS_GetOwnPropertyInternal(tls, "), []byte("_js_backtrace_own_flags(tls, "))
	if n := bytes.Count(fixed, []byte("_js_backtrace_own_flags(tls, ")); n != calls {
		return nil, fmt.Errorf("%s: want %d calls of JS_GetOwnPropertyInternal, found %d", name, calls, n)
	}
	return bytes.Replace(b, body, fixed, 1), nil
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
