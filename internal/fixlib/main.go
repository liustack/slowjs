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
// It does two things:
//
//   - In every lib/ccgo_*.go file, release the string reference that
//     XJS_ToCStringLen2 holds when allocating the UTF-8 copy fails. This is
//     the generated form of the fix in lib/internal/patch/quickjs.diff.
//   - In the hand-written Go files under lib/, replace the import path
//     modernc.org/libquickjs with github.com/liustack/quickjs/lib.
//
// It fails when a generated file no longer has the expected shape, which
// means upstream changed JS_ToCStringLen2 and the fix needs a fresh look.
package main

import (
	"bytes"
	"flag"
	"fmt"
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
		changed, err := update(fn, fixToCString)
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
