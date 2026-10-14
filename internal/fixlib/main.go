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
	"strconv"
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
	reTableEntry     = regexp.MustCompile(`\n\t\*\(\*uintptr\)\(unsafe\.Add\(p, (\d+)\)\) = __ccgo_fp\(_js_module_ns_autoinit\)\n\t\*\(\*uintptr\)\(unsafe\.Add\(p, (\d+)\)\) = __ccgo_fp\(_JS_InstantiateFunctionListItem2\)\n`)
)

const (
	backtraceHelpers = "\n// Added by internal/fixlib for lib/backtrace.go.\n"
	autoinitFree     = "\nfunc _js_autoinit_free(tls *libc.TLS, rt uintptr, pr uintptr) {\n"
	autoinitMark     = "\nfunc _js_autoinit_mark(tls *libc.TLS, rt uintptr, pr uintptr, __ccgo_fp_mark_func uintptr) {\n"
	autoinitTable    = "\nvar _js_autoinit_func_table = [3]uintptr{}\n"
)

// fixBacktrace wires lib/backtrace.go into a generated file:
//
//   - _build_backtrace leaves a stack without a file name to
//     _js_backtrace_lazy, which records it for later.
//   - The autoinit table gets _js_backtrace_autoinit as entry 3,
//     JS_AUTOINIT_ID_BACKTRACE, which formats a recorded stack.
//   - _js_autoinit_free and _js_autoinit_mark also release and mark a
//     recorded stack.
//   - _get_prop_string formats a recorded stack, for printing an error.
//   - At the end, _js_backtrace_prop_flags and _js_backtrace_has_debug read
//     the bit fields JSShapeProperty.flags and JSFunctionBytecode.has_debug,
//     with expressions copied from _get_prop_string and _build_backtrace, as
//     their layout differs between targets.
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
	lazy := fmt.Sprintf("\tif filename == 0 && _js_backtrace_lazy(tls, %s, %s, backtrace_flags) != 0 {\n\t\treturn\n\t}\n", ctx, errorObj)
	if !bytes.Contains(body, []byte(lazy)) {
		init := fmt.Sprintf("\t_js_dbuf_init(tls, %s, bp)\n", ctx)
		if bytes.Count(body, []byte(init)) != 1 {
			return nil, fmt.Errorf("_build_backtrace: want 1 call of _js_dbuf_init")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(init), []byte(lazy+init), 1), 1)
	}

	body, err = funcBody(b, reGetPropString, "_get_prop_string")
	if err != nil {
		return nil, err
	}
	ctx = string(reGetPropString.FindSubmatch(b)[1])
	flags := rePropFlags.FindSubmatch(body)
	value := rePropValue.FindSubmatch(body)
	if flags == nil || value == nil {
		return nil, fmt.Errorf("_get_prop_string: cannot find the property type test or the value")
	}
	pending := fmt.Sprintf("\tif pending := _js_backtrace_prop_string(tls, %s, prs, %s); pending != 0 {\n\t\treturn pending\n\t}\n", ctx, value[1])
	if !bytes.Contains(body, []byte(pending)) {
		b = bytes.Replace(b, body, bytes.Replace(body, flags[1], append([]byte(pending), flags[1]...), 1), 1)
	}

	for _, f := range []struct{ head, call string }{
		{autoinitFree, "\t_js_backtrace_autoinit_free(tls, pr)\n"},
		{autoinitMark, "\t_js_backtrace_autoinit_mark(tls, rt, pr, __ccgo_fp_mark_func)\n"},
	} {
		if bytes.Count(b, []byte(f.head)) != 1 {
			return nil, fmt.Errorf("want 1 %q", strings.TrimSpace(f.head))
		}
		if !bytes.Contains(b, []byte(f.head+f.call)) {
			b = bytes.Replace(b, []byte(f.head), []byte(f.head+f.call), 1)
		}
	}

	if bytes.Contains(b, []byte(autoinitTable)) {
		entries := reTableEntry.FindSubmatch(b)
		if entries == nil {
			return nil, fmt.Errorf("cannot find the entries of _js_autoinit_func_table")
		}
		prev, _ := strconv.Atoi(string(entries[1]))
		last, _ := strconv.Atoi(string(entries[2]))
		b = bytes.Replace(b, []byte(autoinitTable), []byte(strings.Replace(autoinitTable, "[3]", "[4]", 1)), 1)
		b = bytes.Replace(b, entries[0], append(entries[0][:len(entries[0]):len(entries[0])],
			fmt.Sprintf("\t*(*uintptr)(unsafe.Add(p, %d)) = __ccgo_fp(_js_backtrace_autoinit)\n", 2*last-prev)...), 1)
	} else if !bytes.Contains(b, []byte("= __ccgo_fp(_js_backtrace_autoinit)\n")) {
		return nil, fmt.Errorf("cannot find _js_autoinit_func_table")
	}

	var helpers bytes.Buffer
	fmt.Fprintf(&helpers, "package p\n%s", backtraceHelpers)
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_prop_flags(tls *libc.TLS, prs uintptr) int32 {\n\treturn %s\n}\n", flags[2])
	fmt.Fprintf(&helpers, "\nfunc _js_backtrace_has_debug(tls *libc.TLS, b uintptr) int32 {\n\treturn %s\n}\n", hasDebug[1])
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
