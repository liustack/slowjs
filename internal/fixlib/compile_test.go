// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every generated file has each change of fixCompile once, and fixCompile
// leaves a fixed file as it is.
func TestCompileEveryFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixCompile(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixCompile changed a fixed file", fn)
		}
		for _, c := range []struct {
			re string
			n  int
		}{
			{`\)\)\.Fallocated_size = \(\*TDynBuf\)`, 1},
			{`\.Fbyte_code\.Ferror1 != 0 \{\n\t\tXJS_ThrowOutOfMemory\(tls, \w+\)\n\t\tgoto fail\n`, 1},
			{`\n\t+if pos_next < 0 \{\n\t+goto fail\n`, 2},
			{`\nout_of_memory:\n\t;\n\tXJS_ThrowOutOfMemory\(`, 1},
			{`\t+XJS_ThrowOutOfMemory\(tls, \(\*TJSParseState\)\(unsafe\.Pointer\(\w+\)\)\.Fctx\)\n`, 2},
			{`\.Fcur_func\)\)\.Fbyte_code\.Ferror1 != 0 \{`, 1},
			{`\.Ferror1 != 0 \|\| Xdbuf_claim\(tls, bp, `, 1},
			{`\tif _JS_WriteObjectRec\(tls, s, obj\) != 0 \{\n\t\tgoto fail\n\t\}\n\tif \(\*TDynBuf\)\(unsafe\.Pointer\(s\+\d+\)\)\.Ferror1 != 0 \{\n\t\tXJS_ThrowOutOfMemory\(tls, ctx\)\n`, 1},
			{`\.Ferror1 != 0 \{\n\t\t_re_parse_out_of_memory\(tls, \w+\)\n`, 1},
		} {
			if n := len(regexp.MustCompile(c.re).FindAllIndex(out, -1)); n != c.n {
				t.Errorf("%s: %d of %q, want %d", fn, n, c.re, c.n)
			}
		}
	}
}

// undoWriter takes the change to the bytecode writer out of b.
func undoWriter(t *testing.T, b []byte) []byte {
	t.Helper()
	up := regexp.MustCompile(`\(\*TDynBuf\)\(unsafe\.Pointer\(s\+8\)\)\.Ferror1 != 0 \|\| (Xdbuf_claim\(tls, bp, [^\n]*\) != 0 \{\n)\t\tXJS_ThrowOutOfMemory\(tls, [^\n]*\n`).ReplaceAll(b, []byte("$1"))
	up = bytes.Replace(up, []byte("\tif (*TDynBuf)(unsafe.Pointer(s+8)).Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, ctx)\n\t\tgoto fail\n\t}\n"), nil, 1)
	if len(up) == len(b) {
		t.Fatal("nothing undone")
	}
	return up
}

// fixCompile takes a generated file before the change to one after it.
func TestCompileFromUpstream(t *testing.T) {
	b := engineFile(t)
	up := undoWriter(t, b)
	up = bytes.Replace(up, []byte("\t\t\t(*TDynBuf)(unsafe.Pointer(s)).Fallocated_size = (*TDynBuf)(unsafe.Pointer(s)).Fsize\n"), nil, 1)
	up = regexp.MustCompile(`\n\t+if pos_next < 0 \{\n\t+goto fail\n\t+\}`).ReplaceAll(up, nil)

	out, err := fixCompile(up)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, b) {
		t.Fatal("fixCompile does not give the fixed file back")
	}
}

// A writer that no longer calls _JS_WriteObjectRec as it did stops fixlib.
func TestCompileRejectsNewWriter(t *testing.T) {
	up := undoWriter(t, engineFile(t))
	rec := []byte("\tif _JS_WriteObjectRec(tls, s, obj) != 0 {\n")
	if bytes.Count(up, rec) != 1 {
		t.Fatal("cannot find the call of _JS_WriteObjectRec")
	}
	_, err := fixCompile(bytes.Replace(up, rec, []byte("\tif _JS_WriteObjectRec(tls, s, obj, 0) != 0 {\n"), 1))
	if err == nil || !strings.Contains(err.Error(), "XJS_WriteObject2") {
		t.Fatalf("got %v, want XJS_WriteObject2 named", err)
	}
}
