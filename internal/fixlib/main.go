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
// -slots, -frames, -define, -backtrace and -up print the lists that some
// changes are checked against, to update them after a review.
//
// It does fourteen things:
//
//   - In every lib/ccgo_*.go file, release the string reference that
//     XJS_ToCStringLen2 holds when allocating the UTF-8 copy fails. This is
//     the generated form of the fix in lib/internal/patch/quickjs.diff.
//   - In every lib/ccgo_*.go file, wire in the lazy error stacks of
//     lib/backtrace.go, the Go form of the backtrace change in the same
//     diff (see fixBacktrace).
//   - In every lib/ccgo_*.go file, make getTimezoneOffset answer 0, so that
//     javascript's local time is UTC whatever zone the process has, as the
//     same change in lib/internal/patch/quickjs.diff does (see
//     fixTimezone).
//   - In every lib/ccgo_*.go file, keep an uncatchable exception pending
//     until the embedder takes it, so that a script cannot catch it in a
//     promise or an iterator either (see fixUncatchable).
//   - In every lib/ccgo_*.go file, make running out of memory
//     uncatchable, and report every way the engine runs out of memory as
//     the out of memory error (see fixOutOfMemory).
//   - In every lib/ccgo_*.go file, let the compiler run out of memory
//     without crashing or reporting a syntax error (see fixCompile).
//   - In every lib/ccgo_*.go file, give a new runtime the Go port's stack
//     limit, and make running out of stack a RangeError worded as in V8,
//     in the parser too (see fixStack).
//   - In every lib/ccgo_*.go file, put the room of the four calls of alloca
//     into the TLS frame of their function, or malloc it where the function
//     has no frame, so that a call allocates no Go memory and takes as many
//     TLS stack slots as before (see fixAlloca).
//   - In every lib/ccgo_*.go file, replace the TLS frame of the hot small
//     functions of frameSites, which only use it to reinterpret values,
//     with a Go array (see fixFrames).
//   - In every lib/ccgo_*.go file, write each conversion ccgo makes
//     through __ccgo_up as a plain unsafe.Pointer conversion, which keeps
//     the stack frames of the big functions small (see fixUp).
//   - In the lib/ccgo_*.go files of 64-bit targets, keep the JSValue that
//     the interpreter's inlined JS_DupValue and JS_FreeValue go through in
//     a Go variable instead of the TLS frame (see fixCells).
//   - In the lib/ccgo_*.go files of amd64 targets and windows, keep the
//     values the interpreter's loop holds for the whole call in its stack
//     frame, out of the register allocator (see fixPin).
//   - In every lib/ccgo_*.go file, let JS_DefinePropertyValue add a new
//     data property of an ordinary object through objlitDefine of
//     lib/objlit.go, without the three calls of the generic path (see
//     fixDefine).
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
	"crypto/sha256"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
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
	check     = flag.Bool("check", false, "report missing changes without writing files")
	slots     = flag.Bool("slots", false, "print the slot uses of lib/ccgo_darwin_arm64.go in the form of slotSites, to update it after a review")
	define    = flag.Bool("define", false, "print the fingerprints of the functions objlitDefine stands for, in the form of defineSites, to update it after a review")
	frames    = flag.Bool("frames", false, "print the TLS frames of lib/ccgo_darwin_arm64.go and whether each can be dropped, to update frameSites after a review")
	up        = flag.Bool("up", false, "print the statements where fixUp drops an order of reads and calls, in the form of upSites, to update it after a review")
	backtrace = flag.Bool("backtrace", false, "print the fingerprints of the functions lib/backtrace.go repeats, in the form of backtraceSites, to update it after a review")

	// The two allocation failure branches of XJS_ToCStringLen2, with or
	// without the fix already in place.
	reAlloc = regexp.MustCompile(`(\t\tstr_new = _js_alloc_string\(tls, ctx2, [^\n]*, 0\)\n\t\tif !\(str_new != 0\) \{\n)(\t\t\tXFreeValue\(tls, ctx2, [^\n]*\)\n)?(\t\t\tgoto fail\n)`)
	// The fail label of XJS_ToCStringLen2 as generated: it does not free val.
	reToCStringFail = regexp.MustCompile(`\nfail:\n\t;\n\tif plen != 0 \{\n\t\t` + reDeref(`Tsize_t`, `plen`) + ` = uint(32|64)\(0\)\n\t\}\n\treturn libc\.UintptrFromInt32\(0\)$`)
)

func main() {
	flag.Parse()
	if *slots {
		if err := printSlots(filepath.Join("lib", "ccgo_darwin_arm64.go")); err != nil {
			fmt.Fprintln(os.Stderr, "fixlib:", err)
			os.Exit(1)
		}
		return
	}
	if *frames {
		if err := frameReport(filepath.Join("lib", "ccgo_darwin_arm64.go")); err != nil {
			fmt.Fprintln(os.Stderr, "fixlib:", err)
			os.Exit(1)
		}
		return
	}
	if *define || *backtrace || *up {
		files, err := filepath.Glob(filepath.Join("lib", "ccgo_*.go"))
		var hw []*ast.File
		if err == nil {
			hw, err = handWritten("lib")
		}
		if err == nil && *define {
			err = fingerprintReport(files, hw, "defineSites", defineFingerprint)
		} else if err == nil && *backtrace {
			err = fingerprintReport(files, hw, "backtraceSites", backtraceFingerprint)
		} else if err == nil {
			err = upReport(files, hw)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "fixlib:", err)
			os.Exit(1)
		}
		return
	}
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
	hw, err := handWritten("lib")
	if err != nil {
		return 0, err
	}
	for _, fn := range generated {
		changed, err := update(fn, func(b []byte) ([]byte, error) { return fixEngine(b, hw, filepath.Base(fn)) })
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

// fixEngine applies every change to the generated file of lib/ named
// name.
func fixEngine(b []byte, hw []*ast.File, name string) ([]byte, error) {
	b, err := fixBeforeUp(b, hw, name)
	if err != nil {
		return nil, err
	}
	// The changes before fixUp accept both forms of a conversion, so it
	// goes after them.
	if b, err = fixUp(b); err != nil {
		return nil, err
	}
	if b, err = fixCells(b, cellFiles[name]); err != nil {
		return nil, err
	}
	if b, err = fixPin(b, pinFiles[name]); err != nil {
		return nil, err
	}
	// The fingerprints of fixDefine are of the text the changes above
	// leave, so it goes after them.
	if b, err = fixDefine(b, name); err != nil {
		return nil, err
	}
	if err = checkBacktraceSites(b, name); err != nil {
		return nil, err
	}
	// None of the changes after fixBacktrace moves a property slot.
	return b, auditSlots(b)
}

// fixBeforeUp applies the changes that come before fixUp.
func fixBeforeUp(b []byte, hw []*ast.File, name string) ([]byte, error) {
	b, err := fixToCString(b)
	if err != nil {
		return nil, err
	}
	if b, err = fixTimezone(b); err != nil {
		return nil, err
	}
	if b, err = fixUncatchable(b); err != nil {
		return nil, err
	}
	if b, err = fixOutOfMemory(b); err != nil {
		return nil, err
	}
	if b, err = fixCompile(b); err != nil {
		return nil, err
	}
	if b, err = fixStack(b); err != nil {
		return nil, err
	}
	if b, err = fixBacktrace(b); err != nil {
		return nil, err
	}
	if b, err = fixAlloca(b); err != nil {
		return nil, err
	}
	// The TLS frames of darwin/arm64 are checked against frameSites.
	return fixFrames(b, hw, frameSites, name == "ccgo_darwin_arm64.go")
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
// branches of XJS_ToCStringLen2. It relies on upstream not freeing val at
// the fail label, which would make the added call a double free, and on
// four jumps to the label: the failed ToString, the two branches, and the
// one ccgo puts before the label. A change in either stops fixlib.
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
		val = "*(*TJSValue)(unsafe.Pointer(bp + 32))"
	} else if !bytes.Contains(body, []byte("\tvar val, ")) {
		return nil, fmt.Errorf("XJS_ToCStringLen2: cannot locate the local 'val'")
	}

	// A branch that already frees is fixed only when it frees val.
	free := "\t\t\tXFreeValue(tls, ctx2, " + val + ")\n"
	n, other := 0, false
	body = reAlloc.ReplaceAllFunc(body, func(m []byte) []byte {
		n++
		sub := reAlloc.FindSubmatch(m)
		if len(sub[2]) != 0 && string(sub[2]) != free {
			other = true
		}
		return []byte(string(sub[1]) + free + string(sub[3]))
	})
	if n != 2 {
		return nil, fmt.Errorf("XJS_ToCStringLen2: want 2 allocation failure branches, found %d", n)
	}
	if other {
		return nil, fmt.Errorf("XJS_ToCStringLen2: an allocation failure branch frees something other than val")
	}
	if !reToCStringFail.Match(body) {
		return nil, fmt.Errorf("XJS_ToCStringLen2: the fail label changed: if it now frees val, drop fixToCString")
	}
	if n := bytes.Count(body, []byte("goto fail\n")); n != 4 {
		return nil, fmt.Errorf("XJS_ToCStringLen2: want 4 jumps to the fail label, found %d: check that val is freed on the new path", n)
	}

	var out bytes.Buffer
	out.Write(b[:start])
	out.Write(body)
	out.Write(b[end:])
	return out.Bytes(), nil
}

var (
	reTimezoneOffset = regexp.MustCompile(`\nfunc _getTimezoneOffset\(tls \*libc\.TLS, time Tint64_t\) \(r int32\) \{\n`)
	reLocalTime      = regexp.MustCompile(`libc\.X(localtime(_r)?|tzset|mktime|strftime|ctime(_r)?|tzname|timezone|daylight)\b`)
)

const utcOffset = "\nfunc _getTimezoneOffset(tls *libc.TLS, time Tint64_t) (r int32) {\n" +
	"\t// Changed by internal/fixlib: local time is UTC, see lib/internal/patch/quickjs.diff.\n" +
	"\treturn 0\n}\n"

// fixTimezone makes getTimezoneOffset, the engine's only way to the local
// zone, answer 0: javascript's local time is then UTC in every VM, while Go
// and the rest of libc keep the process's zone. The function as generated
// asks libc's localtime (gmtime, mktime and localtime on Windows). Any other
// shape, or a call of libc's local time functions anywhere else in the file,
// means upstream reads the zone some new way and stops fixlib.
func fixTimezone(b []byte) ([]byte, error) {
	body, err := funcBody(b, reTimezoneOffset, "_getTimezoneOffset")
	if err != nil {
		return nil, err
	}
	if string(body) != utcOffset {
		if !bytes.Contains(body, []byte("libc.Xlocaltime")) {
			return nil, fmt.Errorf("_getTimezoneOffset changed shape: it no longer calls libc's localtime")
		}
		b = bytes.Replace(b, body, []byte(utcOffset), 1)
	}
	if m := reLocalTime.Find(b); m != nil {
		return nil, fmt.Errorf("%s outside _getTimezoneOffset: the engine reads the local zone some new way", m)
	}
	return b, nil
}

var (
	reBuildBacktrace = regexp.MustCompile(`\nfunc _build_backtrace\(tls \*libc\.TLS, (\w+) uintptr, (\w+) TJSValue, filename uintptr, line_num int32, col_num int32, backtrace_flags int32\) \{\n`)
	reHasDebug       = regexp.MustCompile(`\n\t+if (int32\(\*\(\*uint8\)\(unsafe\.Pointer\(b \+ \d+\)\)&0x[0-9a-f]+>>\d+\)) != 0 \{\n\t+line_num1 = _find_line_num\(`)
	reGetPropString  = regexp.MustCompile(`\nfunc _get_prop_string\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, prop TJSAtom\) \(r uintptr\) \{\n`)
	rePropFlags      = regexp.MustCompile(`\n(\tif (int32\(\*\(\*uint32\)\(unsafe\.Pointer\(prs \+ 0\)\)&0x[0-9a-f]+>>\d+\))&\(libc\.Int32FromInt32\(3\)<<libc\.Int32FromInt32\(4\)\) != libc\.Int32FromInt32\(0\)<<libc\.Int32FromInt32\(4\) \{\n)`)
	rePropValue      = regexp.MustCompile(`\n\tval = \*\(\*TJSValue\)\(unsafe\.Pointer\((` + reDeref(`uintptr`, `bp(?: \+ \d+)?`) + `)\)\)\n`)
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
	// pending stack. Its value pointer is the pr of the lookup: a local, or
	// its frame slot read before or after fixUp. The test of a VARREF
	// property reads Fpvalue instead, and after fixUp has the same form.
	reDefineHead = regexp.MustCompile(`\nfunc XJS_DefineProperty\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, prop TJSAtom, \w+ TJSValue, \w+ TJSValue, \w+ TJSValue, flags int32\) \(r int32\) \{\n`)
	reDefineFind = regexp.MustCompile(`\n[ \t]+([^\n=]+?) = _find_own_property\(tls, [^,\n]+, (\w+), prop\)\n`)
	reDefineSame = regexp.MustCompile(`\n([ \t]+)if !\(_js_same_value\(tls, \w+, \w+, \*\(\*TJSValue\)\(unsafe\.Pointer\((\w+|\*\*\(\*\*uintptr\)\(__ccgo_up\(bp \+ \d+\)\)|\*\(\*uintptr\)\(unsafe\.Pointer\(bp \+ \d+\)\))\)\)\) != 0\) \{\n`)

	// The two printer sites: js_print_error reads the stack, js_print_object
	// prints a normal property value.
	rePrintErrorHead     = regexp.MustCompile(`\nfunc _js_print_error\(tls \*libc\.TLS, s uintptr, p uintptr\) \{\n`)
	rePrintObjectHead    = regexp.MustCompile(`\nfunc _js_print_object\(tls \*libc\.TLS, s uintptr, \w+ uintptr\) \{\n`)
	rePrintObjectWrapped = regexp.MustCompile(`\n(\t+)if _js_backtrace_print_pending\(tls, s, prs, pr\) == 0 \{\n\t+_js_print_value\(tls, s, \*\(\*TJSValue\)\(unsafe\.Pointer\(pr\)\)\)\n\t+\}\n`)
	rePrintObjectVal     = regexp.MustCompile(`\n(\t+)_js_print_value\(tls, s, \*\(\*TJSValue\)\(unsafe\.Pointer\(pr\)\)\)\n`)
)

const (
	backtraceHelpers = "\n// Added by internal/fixlib for lib/backtrace.go.\n"
	memoryUsage      = "_js_backtrace_memory_usage(tls, s, hp, prs, pr)\n"
)

// slotDeref renders the load of a find_own_property storage slot, which
// the generated code writes as "bp" or "bp+NN", in the form fixUp leaves.
func slotDeref(arg string) string {
	return "*(*uintptr)(unsafe.Pointer(" + strings.Replace(arg, "+", " + ", 1) + "))"
}

// reDeref is a regular expression for the load of a typ at the address
// that arg matches, in either form of the conversion (see fixUp). It adds
// no capturing group to those of arg.
func reDeref(typ, arg string) string {
	return `(?:\*\*\(\*\*` + typ + `\)\(__ccgo_up|\*\(\*` + typ + `\)\(unsafe\.Pointer)\(` + arg + `\)\)`
}

// backtraceFuncs are the functions whose work lib/backtrace.go repeats for
// a pending stack: _build_backtrace walks the frames and formats the text
// (backtraceCount, backtraceRecord, backtraceFormat), _get_prop_string
// reads a function's name (backtraceFuncName) and _JS_GetFunctionBytecode
// finds its bytecode.
var backtraceFuncs = []string{"_build_backtrace", "_get_prop_string", "_JS_GetFunctionBytecode"}

var (
	reLazyBacktrace   = regexp.MustCompile(`\tif filename == 0 && _js_backtrace_build\(tls, [^\n]*\{\n\t\treturn\n\t\}\n`)
	reStringUse       = regexp.MustCompile(`__ccgo_ts\+(\d+)\b`)
	rePendingPropText = regexp.MustCompile(`\tif pending, ok := _js_backtrace_prop_string\(tls, [^\n]*\{\n\t\treturn pending\n\t\}\n`)
)

// backtraceFingerprint hashes backtraceFuncs, without the calls
// fixBacktrace adds, with each C string written out in place of its
// offset, which a new string anywhere before it moves.
func backtraceFingerprint(b []byte) (string, error) {
	m := reStringTable.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("cannot find __ccgo_ts1")
	}
	table, err := strconv.Unquote(string(m[1]))
	if err != nil {
		return "", fmt.Errorf("__ccgo_ts1: %v", err)
	}
	h := sha256.New()
	for _, name := range backtraceFuncs {
		text, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		text = reLazyBacktrace.ReplaceAll(text, nil)
		text = rePendingPropText.ReplaceAll(text, nil)
		text = reStringUse.ReplaceAllFunc(text, func(use []byte) []byte {
			off, _ := strconv.Atoi(string(reStringUse.FindSubmatch(use)[1]))
			if off >= len(table) {
				return use
			}
			str, _, _ := strings.Cut(table[off:], "\x00")
			return []byte(strconv.Quote(str))
		})
		h.Write(text)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// checkBacktraceSites checks backtraceFuncs against backtraceSites. The
// fingerprints are of the text after every other change.
func checkBacktraceSites(b []byte, file string) error {
	got, err := backtraceFingerprint(b)
	if err != nil {
		return err
	}
	if want := backtraceSites[file]; got != want && !reporting() {
		return fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/backtrace.go, which must format a pending stack with the same bytes, then update backtraceSites (go run ./internal/fixlib -backtrace)", file, strings.Join(backtraceFuncs, ", "), got, want)
	}
	return nil
}

// afterFirst is a place for insertAt: right after the first match of re.
func afterFirst(re *regexp.Regexp, msg string) func([]byte) (int, error) {
	return func(body []byte) (int, error) {
		loc := re.FindIndex(body)
		if loc == nil {
			return 0, fmt.Errorf("%s", msg)
		}
		return loc[1], nil
	}
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
// auditSlots then checks every use of a property slot against slotSites
// (see slots.go), so that new or changed code upstream that uses a slot
// stops fixlib until someone has reviewed it. At the end it adds
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
	fixed, err := insertAt("_build_backtrace", body, lazy, func(body []byte) (int, error) {
		return before(body, fmt.Sprintf("\t_js_dbuf_init(tls, %s, bp)\n", ctx), "", "_build_backtrace: want 1 call of _js_dbuf_init")
	})
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

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
	if fixed, err = insertAt("_get_prop_string", body, pending, func(body []byte) (int, error) {
		loc := rePropFlags.FindSubmatchIndex(body)
		if loc == nil {
			return 0, fmt.Errorf("_get_prop_string: cannot find the property type test")
		}
		return loc[2], nil
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// JS_GetPropertyInternal: settle the stack before the flag dispatch.
	if body, err = funcBody(b, reGetPropHead, "XJS_GetPropertyInternal"); err != nil {
		return nil, err
	}
	getCtx := string(reGetPropHead.FindSubmatch(b)[1])
	fm := reGetPropFind.FindSubmatch(body)
	if fm == nil {
		return nil, fmt.Errorf("XJS_GetPropertyInternal: cannot find the property lookup")
	}
	ins := fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, prs, %s) < 0 {\n\t\t\t\treturn jsvException()\n\t\t\t}\n\t\t}\n", getCtx, string(fm[3]), slotDeref(string(fm[2])))
	if fixed, err = insertAt("XJS_GetPropertyInternal", body, ins, afterFirst(reGetPropFind, "XJS_GetPropertyInternal: cannot find the property lookup")); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// JS_GetOwnPropertyInternal: settle the stack before reading the value.
	if body, err = funcBody(b, reGetOwnHead, "_JS_GetOwnPropertyInternal"); err != nil {
		return nil, err
	}
	om := reGetOwnHead.FindSubmatch(b)
	ownCtx, ownObj := string(om[1]), string(om[2])
	if fm = reGetOwnFind.FindSubmatch(body); fm == nil {
		return nil, fmt.Errorf("_JS_GetOwnPropertyInternal: cannot find the property lookup")
	}
	ins = fmt.Sprintf("\t\tif prop == uint32(EJS_ATOM_stack) && desc != 0 {\n\t\t\tif _js_backtrace_settle(tls, %s, %s, prs, %s) < 0 {\n\t\t\t\treturn -int32(1)\n\t\t\t}\n\t\t}\n", ownCtx, ownObj, slotDeref(string(fm[2])))
	if fixed, err = insertAt("_JS_GetOwnPropertyInternal", body, ins, afterFirst(reGetOwnFind, "_JS_GetOwnPropertyInternal: cannot find the property lookup")); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

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
	sameAt := func(body []byte) ([]int, error) {
		same := reDefineSame.FindAllSubmatchIndex(body, -1)
		if len(same) != 1 {
			return nil, fmt.Errorf("XJS_DefineProperty: want 1 SameValue test of a data property, found %d", len(same))
		}
		return same[0], nil
	}
	same, err := sameAt(body)
	if err != nil {
		return nil, err
	}
	indent := string(body[same[2]:same[3]])
	pr := string(body[same[4]:same[5]])
	ins = fmt.Sprintf("\n%sif prop == uint32(EJS_ATOM_stack) && _js_backtrace_settle(tls, %s, %s, %s, %s) < 0 {\n%s\treturn -int32(1)\n%s}", indent, defCtx, objs[0][2], objs[0][1], pr, indent, indent)
	if fixed, err = insertAt("XJS_DefineProperty", body, ins, func(body []byte) (int, error) {
		same, err := sameAt(body)
		if err != nil {
			return 0, err
		}
		return same[0], nil
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// The callers of JS_GetOwnPropertyInternal that only look at the
	// attributes call _js_backtrace_own_flags, which does not format.
	for _, fn := range ownFlagsCallers {
		if b, err = fixOwnFlags(b, fn.name, fn.calls, fn.values); err != nil {
			return nil, err
		}
	}

	// js_print_error: skip the stack of a raw dump.
	if body, err = funcBody(b, rePrintErrorHead, "_js_print_error"); err != nil {
		return nil, err
	}
	// The line that reads the stack, and its indent.
	stackRead := func(body []byte) (int, string, error) {
		end := bytes.Index(body, []byte(", uint32(EJS_ATOM_stack))"))
		if end < 0 {
			return 0, "", fmt.Errorf("_js_print_error: cannot find the stack read")
		}
		start := bytes.LastIndex(body[:end], []byte("str = _get_prop_string(tls,"))
		if start < 0 {
			return 0, "", fmt.Errorf("_js_print_error: cannot find the stack read start")
		}
		ls := bytes.LastIndexByte(body[:start], '\n') + 1
		return ls, string(body[ls:start]), nil
	}
	_, indent, err = stackRead(body)
	if err != nil {
		return nil, err
	}
	guard := fmt.Sprintf("%sif _js_backtrace_raw_stack(tls, s, p) {\n%s\treturn\n%s}\n", indent, indent, indent)
	if fixed, err = insertAt("_js_print_error", body, guard, func(body []byte) (int, error) {
		ls, _, err := stackRead(body)
		return ls, err
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// js_print_object: a placeholder for a marker value.
	if body, err = funcBody(b, rePrintObjectHead, "_js_print_object"); err != nil {
		return nil, err
	}
	if n := bytes.Count(body, []byte("_js_backtrace_print_pending")); n != 0 {
		if n != 1 || len(rePrintObjectWrapped.FindAllIndex(body, -1)) != 1 {
			return nil, fmt.Errorf("_js_print_object: the call of _js_backtrace_print_pending is not around the value print")
		}
	} else {
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
	propSize := func(body []byte) ([]int, error) {
		loc := reMemoryProp.FindAllSubmatchIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("XJS_ComputeMemoryUsage: want 1 size of a property value, found %d", len(loc))
		}
		return loc[0], nil
	}
	loc, err := propSize(body)
	if err != nil {
		return nil, err
	}
	if fixed, err = insertAt("XJS_ComputeMemoryUsage", body, string(body[loc[2]:loc[3]])+memoryUsage, func(body []byte) (int, error) {
		loc, err := propSize(body)
		if err != nil {
			return 0, err
		}
		return loc[1], nil
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	if !*slots {
		if err := auditSlots(b); err != nil {
			return nil, err
		}
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
		pr := slotDeref(string(fixed[s[4]:s[5]]))
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
// stack. values is the number of reads of the value of such a descriptor:
// those of XJS_SetPropertyInternal free it (JS_FreeValue inlined). Any other
// use of the descriptor than reading its attributes, freeing it or passing
// it to the exotic get_own_property stops fixlib (see descUses).
var ownFlagsCallers = []struct {
	name   string
	calls  int
	values int
}{
	{"_JS_GetOwnPropertyNamesInternal", 1, 0},  // enumerability of exotic keys
	{"XJS_SetPropertyInternal", 1, 2},          // writable before a redefine
	{"_JS_CopyDataProperties", 2, 0},           // enumerability, values read by JS_GetProperty
	{"_JS_GetOwnPropertyNames2", 1, 0},         // enumerability, values read by JS_GetProperty
	{"_js_object_seal", 1, 0},                  // writable, for freeze
	{"_js_object_isSealed", 1, 0},              // configurable and writable
	{"_js_object_propertyIsEnumerable", 1, 0},  // enumerable
	{"_js_object___lookupGetter__", 1, 0},      // getter or setter
	{"_js_proxy_has", 1, 0},                    // configurable
	{"_js_proxy_delete_property", 1, 0},        // configurable
	{"_js_proxy_get_own_property_names", 1, 0}, // configurable
}

func fixOwnFlags(b []byte, name string, calls, values int) ([]byte, error) {
	re := regexp.MustCompile(`\nfunc ` + regexp.QuoteMeta(name) + `\(tls \*libc\.TLS, [^\n]*\{\n`)
	body, err := funcBody(b, re, name)
	if err != nil {
		return nil, err
	}
	fixed := bytes.ReplaceAll(body, []byte("_JS_GetOwnPropertyInternal(tls, "), []byte("_js_backtrace_own_flags(tls, "))
	if n := bytes.Count(fixed, []byte("_js_backtrace_own_flags(tls, ")); n != calls {
		return nil, fmt.Errorf("%s: want %d calls of JS_GetOwnPropertyInternal, found %d", name, calls, n)
	}
	uses, err := descUses(name, fixed)
	if err != nil {
		return nil, err
	}
	for use, n := range uses {
		switch use {
		case "call _js_backtrace_own_flags", "call _js_free_desc", "call indirect":
		case "value":
			if n != values {
				return nil, fmt.Errorf("%s: %d reads of the value of a descriptor from JS_GetOwnPropertyInternal, want %d: a caller that reads the value must not get _js_backtrace_own_flags", name, n, values)
			}
		default:
			return nil, fmt.Errorf("%s: a descriptor from JS_GetOwnPropertyInternal is used some other way (%s): a caller that reads the value must not get _js_backtrace_own_flags", name, use)
		}
	}
	if uses["value"] != values {
		return nil, fmt.Errorf("%s: %d reads of the value of a descriptor from JS_GetOwnPropertyInternal, want %d", name, uses["value"], values)
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// descUses counts how the function in body uses the descriptors it passes
// to _js_backtrace_own_flags, cells bp+K of its TLS frame: "value" for a
// read of the value field, "raw" for an address in the cell (bp + K + N,
// or bp + N up to where the value ends),
// "call f" for a call it is passed to, and "other" for anything else. Reads
// of the flags, the getter and the setter are not counted.
func descUses(name string, body []byte) (map[string]int, error) {
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, err
	}
	cells := map[int]bool{}
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && isIdent(c.Fun, "_js_backtrace_own_flags") && len(c.Args) == 5 {
			if k, ok := bpOffset(c.Args[2]); ok {
				cells[k] = true
			}
		}
		return true
	})
	uses := map[string]int{}
	var stack []ast.Node
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		k, ok := bpOffset(e)
		if !ok {
			return true
		}
		if a, ok := stack[len(stack)-2].(*ast.AssignStmt); ok && a.Tok == token.DEFINE {
			return false // bp := tls.Alloc(N)
		}
		if cells[k] {
			if use := descUse(stack); use != "" {
				uses[use]++
			}
			return false
		}
		for c := range cells {
			if c < k && k < c+16 { // the flags and the value
				uses["raw"]++
			}
		}
		return false
	})
	return uses, nil
}

// bpOffset returns K for bp or bp + K.
func bpOffset(e ast.Expr) (int, bool) {
	if isIdent(e, "bp") {
		return 0, true
	}
	if be, ok := e.(*ast.BinaryExpr); ok && be.Op == token.ADD && isIdent(be.X, "bp") {
		return intLit(be.Y)
	}
	return 0, false
}

// descUse names the use of the descriptor at the top of stack for descUses.
func descUse(stack []ast.Node) string {
	parent := func(i int) ast.Node {
		if i < 0 {
			return nil
		}
		return stack[i]
	}
	i := len(stack) - 2
	switch p := parent(i).(type) {
	case *ast.BinaryExpr:
		if n, ok := intLit(p.Y); ok && n >= 16 {
			return "" // the getter or the setter
		}
		return "raw"
	case *ast.CallExpr:
		if up := isIdent(p.Fun, "__ccgo_up"); up || isSelector(p.Fun, "unsafe", "Pointer") {
			// (*(*TJSPropertyDescriptor)(unsafe.Pointer(d))).F, before fixUp
			// (**(**TJSPropertyDescriptor)(__ccgo_up(d))).F, or either
			// without the outer parentheses.
			stars := 1
			if up {
				stars = 2
			}
			conv, ok := parent(i - 1).(*ast.CallExpr)
			if !ok {
				return "other"
			}
			typ, ok := conv.Fun.(*ast.ParenExpr)
			if !ok {
				return "other"
			}
			t := typ.X
			for range stars {
				st, ok := t.(*ast.StarExpr)
				if !ok {
					return "other"
				}
				t = st.X
			}
			if !isIdent(t, "TJSPropertyDescriptor") {
				return "other"
			}
			j := i - 2
			for range stars {
				if _, ok := parent(j).(*ast.StarExpr); !ok {
					return "other"
				}
				j--
			}
			if _, ok := parent(j).(*ast.ParenExpr); ok {
				j--
			}
			sel, ok := parent(j).(*ast.SelectorExpr)
			if !ok {
				return "other"
			}
			switch sel.Sel.Name {
			case "Fflags", "Fgetter", "Fsetter":
				return ""
			case "Fvalue":
				return "value"
			}
			return "other"
		}
		switch fun := p.Fun.(type) {
		case *ast.Ident:
			return "call " + fun.Name
		case *ast.ParenExpr:
			return "call indirect"
		}
		return "other"
	}
	return "other"
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
