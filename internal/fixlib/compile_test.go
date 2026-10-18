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
			{`\.Fbyte_code\.Ferror1 != 0 \{\n\t\tXJS_ThrowOutOfMemory\(tls, \w+\)\n\t\tgoto fail\n`, 2},
			{`\n\t+if pos_next < 0 \{\n\t+goto fail\n`, 2},
			{`\nout_of_memory:\n\t;\n\tXJS_ThrowOutOfMemory\(`, 1},
			{`\nfail:\n\t;\n\t/\* free the relocations of the labels not reached \*/\n`, 1},
			{`\t+XJS_ThrowOutOfMemory\(tls, \(\*TJSParseState\)\(unsafe\.Pointer\(\w+\)\)\.Fctx\)\n`, 5},
			{`\.Fcur_func\)\)\.Fbyte_code\.Ferror1 != 0 \{`, 3},
			{`\.Ferror1 != 0 \|\| Xdbuf_claim\(tls, bp, `, 1},
			{`\tif _JS_WriteObjectRec\(tls, s, obj\) != 0 \{\n\t\tgoto fail\n\t\}\n\tif \(\*TDynBuf\)\(unsafe\.Pointer\(s\+\d+\)\)\.Ferror1 != 0 \{\n\t\tXJS_ThrowOutOfMemory\(tls, ctx\)\n`, 1},
			{`\.Ferror1 != 0 \{\n\t\t_re_parse_out_of_memory\(tls, \w+\)\n`, 1},
			{`\.Fbyte_code\.Ferror1 != 0 \{\n\t\t\tXJS_ThrowOutOfMemory\(tls, \(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fctx\)\n\t\t\treturn -int32\(1\)\n`, 2},
			{`\n\tif \(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fcur_func != 0 && [^\n]*\n\t\tXJS_ThrowOutOfMemory\([^\n]*\n\t\treturn -int32\(1\)\n\t\}\n\t_free_token\(`, 1},
			{`\.Ffields_init_fd\)\)\.Fbyte_code\.Ferror1 != 0 \{\n\t\t\tXJS_ThrowOutOfMemory\(tls, ctx\)\n\t\t\tgoto fail\n`, 1},
			{`\(\*TDynBuf\)\(unsafe\.Pointer\(bc\)\)\.Ferror1 != 0 \|\| Xdbuf_claim\(tls, bc, `, 2},
			{`\.Fdebug\.Fpc2line_buf = Xjs_realloc_rt\(tls, `, 1},
			{`\.Fdebug\.Fpc2line_buf = Xjs_realloc\(`, 0},
			{`\t\tif _bc_atom_to_idx\(tls, s, bp, atom\) != 0 \{\n\t\t\t\(\*TBCWriterState\)\(unsafe\.Pointer\(s\)\)\.Fdbuf\.Ferror1 = int32\(1\)\n`, 1},
			{`\.Fbyte_code\.Ferror1 = int32\(1\)\n\t+return -int32\(1\)\n`, 2},
			{`\.Fbyte_code\.Ferror1 != 0 \{\n\t\t\treturn\n`, 1},
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
	up = undoParserStops(t, up)
	up = bytes.Replace(up, []byte("\t\t\t(*TBCWriterState)(unsafe.Pointer(s)).Fdbuf.Ferror1 = int32(1)\n"), nil, 1)
	up = regexp.MustCompile(`(\.Fdebug\.Fpc2line_buf = )Xjs_realloc_rt\(tls, \(\*TJSContext\)\(unsafe\.Pointer\((\w+)\)\)\.Frt, `).ReplaceAll(up, []byte("${1}Xjs_realloc(tls, $2, "))

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

// undoParserStops takes out of b the checks that stop the parser once the
// code is lost.
func undoParserStops(t *testing.T, b []byte) []byte {
	t.Helper()
	n := len(b)
	b = regexp.MustCompile(`\n\tif \(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fcur_func != 0 && [^\n]*\n\t\tXJS_ThrowOutOfMemory\([^\n]*\n\t\treturn -int32\(1\)\n\t\}(\n\t_free_token\()`).ReplaceAll(b, []byte("$1"))
	b = regexp.MustCompile(`(\n\t/\* patch the constant pool index for the constructor \*/)\n\tif [^\n]*\n\t\tXJS_ThrowOutOfMemory\(tls, ctx\)\n\t\tgoto fail\n\t\}`).ReplaceAll(b, []byte("$1"))
	b = regexp.MustCompile(`\n\t\tif [^\n]*\.Ffields_init_fd\)\)\.Fbyte_code\.Ferror1 != 0 \{\n\t\t\tXJS_ThrowOutOfMemory\(tls, ctx\)\n\t\t\tgoto fail\n\t\t\}`).ReplaceAll(b, nil)
	b = regexp.MustCompile(`(\n\t\t/\* remove test and decrement label ref count \*/)\n\t\tif [^\n]*\n\t\t\tXJS_ThrowOutOfMemory\([^\n]*\n\t\t\treturn -int32\(1\)\n\t\t\}`).ReplaceAll(b, []byte("$1"))
	b = bytes.ReplaceAll(b, []byte("(*TDynBuf)(unsafe.Pointer(bc)).Ferror1 != 0 || Xdbuf_claim("), []byte("Xdbuf_claim("))
	if n-len(b) < 5*40 {
		t.Fatal("not all undone")
	}
	return b
}

// A dbuf_claim that no longer fails at once on a failed buffer stops
// fixlib: the full buffer fixCompile leaves would grow again, and a later
// write would follow the one that failed.
func TestCompileRejectsClaimAfterFailure(t *testing.T) {
	b := engineFile(t)
	test := "\t\tif (*TDynBuf)(unsafe.Pointer(s)).Ferror1 != 0 {\n\t\t\treturn -int32(1)\n\t\t}\n"
	body, err := funcBody(b, reDbufClaim, "Xdbuf_claim")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(body, []byte(test)) != 1 {
		t.Fatal("cannot find the test of a failed buffer")
	}
	changed := bytes.Replace(b, body, bytes.Replace(body, []byte(test), nil, 1), 1)
	if _, err := fixCompile(changed); err == nil {
		t.Fatal("a dbuf_claim without the test went through")
	}
}

// moveLines takes the n lines starting with the first line of the function
// head that contains first, and puts them right after the head of the
// function: a file where upstream wrote fixlib's text somewhere else.
func moveLines(t *testing.T, b []byte, head, first string, n int) []byte {
	t.Helper()
	body, err := funcBody(b, funcHead(head), head)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(body, []byte(first))
	if i < 0 {
		t.Fatalf("no %q in %s", first, head)
	}
	start := bytes.LastIndexByte(body[:i], '\n') + 1
	end := start
	for range n {
		end += bytes.IndexByte(body[end:], '\n') + 1
	}
	block := body[start:end]
	rest := append(append([]byte{}, body[:start]...), body[end:]...)
	at := bytes.IndexByte(rest[1:], '\n') + 2 // after the head line
	moved := append(append(append([]byte{}, rest[:at]...), block...), rest[at:]...)
	return bytes.Replace(b, body, moved, 1)
}

// A change of fixCompile counts as made only as the whole text fixlib
// writes, where fixlib writes it. The same text somewhere else in the
// function, as upstream might write part of the change, stops fixlib.
func TestCompileRejectsTextElsewhere(t *testing.T) {
	b := engineFile(t)
	for _, c := range []struct {
		head, first string
		n           int
	}{
		{"Xdbuf_claim", "Fallocated_size = (*TDynBuf)(unsafe.Pointer(s)).Fsize", 1},
		{"_js_create_function", "if (*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1 != 0 {", 4},
		{"_resolve_variables", "if pos_next < 0 {", 3},
		{"_js_parse_for_in_of", "XJS_ThrowOutOfMemory(tls, (*TJSParseState)(unsafe.Pointer(s)).Fctx)", 1},
		{"_js_parse_statement_or_decl", "XJS_ThrowOutOfMemory(tls, (*TJSParseState)(unsafe.Pointer(s)).Fctx)", 1},
		{"_js_parse_error_v", "if (*TJSParseState)(unsafe.Pointer(s)).Fcur_func != 0 &&", 4},
		{"Xlre_compile", "if *(*byte)(unsafe.Pointer(s4+168)) == 0 ||", 3},
		{"_resolve_labels", "if (*TDynBuf)(unsafe.Pointer(bp+8)).Ferror1 != 0 {", 3},
		{"_resolve_labels", "/* free the relocations of the labels not reached */", 10},
		{"_JS_WriteObjectAtoms", "XJS_ThrowOutOfMemory(tls, (*TBCWriterState)(unsafe.Pointer(s)).Fctx)", 1},
		{"_bc_put_atom", "(*TBCWriterState)(unsafe.Pointer(s)).Fdbuf.Ferror1 = int32(1)", 1},
		{"XJS_WriteObject2", "if (*TDynBuf)(unsafe.Pointer(s+8)).Ferror1 != 0 {", 4},
		{"_push_scope", "if (*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1 != 0 {", 4},
		{"_pop_scope", "if (*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1 != 0 {", 3},
		{"_js_parse_class", "if (*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1 != 0 {", 4},
		{"_js_parse_class", "Ffields_init_fd)).Fbyte_code.Ferror1 != 0 {", 4},
		{"_js_parse_destructuring_element", "Fcur_func)).Fbyte_code.Ferror1 != 0 {", 4},
		{"_next_token", "if (*TJSParseState)(unsafe.Pointer(s)).Fcur_func != 0 &&", 4},
	} {
		if _, err := fixCompile(moveLines(t, b, c.head, c.first, c.n)); err == nil {
			t.Errorf("%s: %q elsewhere went through", c.head, c.first)
		}
	}
}

// A change fixCompile makes at several places is checked at each: one
// place left as upstream wrote it gets the change, whatever the others.
func TestCompileFixesEachPlace(t *testing.T) {
	b := engineFile(t)
	// The second check of _resolve_variables gone.
	body, err := funcBody(b, funcHead("_resolve_variables"), "_resolve_variables")
	if err != nil {
		t.Fatal(err)
	}
	check := "\t\tif pos_next < 0 {\n\t\t\tgoto fail\n\t\t}\n"
	i := bytes.LastIndex(body, []byte(check))
	one := append(append([]byte{}, body[:i]...), body[i+len(check):]...)
	out, err := fixCompile(bytes.Replace(b, body, one, 1))
	if err != nil || !bytes.Equal(out, b) {
		t.Errorf("_resolve_variables with one check: %v", err)
	}

	// The line table shrunk with Xjs_realloc again, and the new form
	// somewhere else.
	shrink := "Xjs_realloc_rt(tls, (*TJSContext)(unsafe.Pointer(ctx)).Frt, (*TJSFunctionDef)(unsafe.Pointer(fd)).Fpc2line.Fbuf, "
	old := "Xjs_realloc(tls, ctx, (*TJSFunctionDef)(unsafe.Pointer(fd)).Fpc2line.Fbuf, "
	changed := bytes.Replace(b, []byte(shrink), []byte(old), 1)
	body, err = funcBody(changed, funcHead("_js_create_function"), "_js_create_function")
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.IndexByte(body[1:], '\n') + 2
	changed = bytes.Replace(changed, body, append(append(append([]byte{}, body[:at]...), "\t(*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf = "+shrink+"0)\n"...), body[at:]...), 1)
	out, err = fixCompile(changed)
	if err == nil && bytes.Contains(out, []byte(old)) {
		t.Error("the line table is still shrunk with Xjs_realloc")
	}
}

// A label out_of_memory of upstream's own in _resolve_labels stops fixlib:
// the change uses that name.
func TestCompileRejectsOwnLabel(t *testing.T) {
	b := undoResolveLabels(t, engineFile(t))
	body, err := funcBody(b, funcHead("_resolve_labels"), "_resolve_labels")
	if err != nil {
		t.Fatal(err)
	}
	at := bytes.Index(body, []byte("\tpos = 0\n\tfor {\n"))
	changed := append(append(append([]byte{}, body[:at]...), "\tgoto out_of_memory\nout_of_memory:\n\t;\n"...), body[at:]...)
	if _, err := fixCompile(bytes.Replace(b, body, changed, 1)); err == nil {
		t.Error("a label out_of_memory of upstream went through")
	}
}

// undoResolveLabels gives _resolve_labels of b back the form ccgo writes.
func undoResolveLabels(t *testing.T, b []byte) []byte {
	t.Helper()
	body, err := funcBody(b, funcHead("_resolve_labels"), "_resolve_labels")
	if err != nil {
		t.Fatal(err)
	}
	up := bytes.Replace(body, []byte("\t\tif (*TDynBuf)(unsafe.Pointer(bp+8)).Ferror1 != 0 {\n\t\t\tgoto out_of_memory\n\t\t}\n"), nil, 1)
	up = bytes.Replace(up, []byte("\tif (*TDynBuf)(unsafe.Pointer(bp+8)).Ferror1 != 0 {\n\t\tgoto out_of_memory\n\t}\n"), nil, 1)
	i := bytes.Index(up, []byte("out_of_memory:\n"))
	if i < 0 || bytes.Contains(up, []byte("goto out_of_memory")) {
		t.Fatal("cannot undo _resolve_labels")
	}
	up = append(up[:i:i], "fail:\n\t;\n\t/* XXX: not safe */\n\tXdbuf_free(tls, bp+8)\n\treturn -int32(1)\n}\n"...)
	out := bytes.Replace(b, body, up, 1)
	if got, err := fixCompile(out); err != nil || !bytes.Equal(got, b) {
		t.Fatalf("undoResolveLabels: fixCompile does not give the file back: %v", err)
	}
	return out
}

// A new jump to the fail label of _resolve_labels stops fixlib, in the form
// ccgo writes and in a fixed file: the end fixlib writes keeps the new code,
// which is right only for the four failed relocations it was checked for.
func TestCompileRejectsNewFailJump(t *testing.T) {
	fixed := engineFile(t)
	for name, b := range map[string][]byte{"upstream": undoResolveLabels(t, fixed), "fixed": fixed} {
		body, err := funcBody(b, funcHead("_resolve_labels"), "_resolve_labels")
		if err != nil {
			t.Fatal(err)
		}
		at := bytes.Index(body, []byte("\tpos = 0\n\tfor {\n"))
		changed := append(append(append([]byte{}, body[:at]...), "\tif bc_len == 0 {\n\t\tgoto fail\n\t}\n"...), body[at:]...)
		if _, err := fixCompile(bytes.Replace(b, body, changed, 1)); err == nil {
			t.Errorf("%s: a new goto fail went through", name)
		}
	}
}
