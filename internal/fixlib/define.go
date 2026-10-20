// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// fixDefine makes JS_DefinePropertyValue try objlitDefine of
// lib/objlit.go first, and gives each generated file the JSObject bit
// fields objlitDefine reads, taken from the file's own
// JS_NewObjectFromShape, so that the layout of every target (the 32-bit
// ones, big-endian s390x) comes from ccgo and not from a table.
//
// objlitDefine repeats what JS_DefinePropertyValue, JS_DefineProperty,
// JS_CreateProperty and add_property do for a new data property of an
// ordinary object. So the generated text of those functions, of
// find_own_property, which JS_DefineProperty calls, and of
// find_own_property1, which objlitDefine calls instead, is pinned in
// defineSites: when a release changes one of them, fixDefine stops, and the
// new code must be read against objlitDefine before the fingerprints are
// updated (go run ./internal/fixlib -define prints them). The fingerprints are of the text
// after every other change, so fixDefine runs after them.
func fixDefine(b []byte, file string) ([]byte, error) {
	all := findAll(reDefineValueHead, b)
	if len(all) == 0 {
		return nil, fmt.Errorf("JS_DefinePropertyValue changed shape: its signature is not the expected one")
	}
	if len(all) != 1 {
		return nil, fmt.Errorf("JS_DefinePropertyValue: want one definition, found %d", len(all))
	}
	m := all[0]
	call := []byte(fmt.Sprintf(defineCall, b[m[2]:m[3]]))
	if head := m[1]; !bytes.HasPrefix(b[head:], call) {
		// Before the TLS frame (64-bit targets), so that the fast path does
		// not pay for it.
		b = append(b[:head:head], append(call, b[head:]...)...)
	}

	got, err := defineFingerprint(b)
	if err != nil {
		return nil, err
	}
	// -define reports the fingerprints instead (see defineReport).
	if want := defineSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: JS_DefinePropertyValue, JS_DefineProperty, JS_CreateProperty, add_property, find_own_property or find_own_property1 changed (fingerprint %s, want %s): read them against lib/objlit.go, then update defineSites", file, got, want)
	}

	layout, err := objectBits(b)
	if err != nil {
		return nil, err
	}
	if i := bytes.Index(b, []byte(layoutMark)); i >= 0 {
		end := bytes.Index(b[i:], []byte("\n)\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of the JSObject bit field constants not found")
		}
		b = append(b[:i:i], b[i+end+3:]...)
	}
	return append(b, layout...), nil
}

var (
	reDefineValueHead = regexp.MustCompile(`\nfunc XJS_DefinePropertyValue\(tls \*libc\.TLS, (ctx\d*) uintptr, this_obj TJSValue, prop TJSAtom, val TJSValue, flags int32\) \(r int32\) \{\n`)
	// The bit fields of JSObject in the order JS_NewObjectFromShape sets
	// them: is_std_array_prototype, extensible, free_mark, is_exotic,
	// fast_array, ...
	reObjectBits = regexp.MustCompile(`libc\.SetBitFieldPtr8Uint8\(p\+(\d+), [^\n]*?, (\d+), (0x[0-9a-f]+)\)\n`)
	// The bit tests of the generic path, each giving the offset, the mask
	// and the shift: is_exotic, the first test of JS_CreateProperty;
	// extensible, the test before its jump to not_extensible for the
	// objects that are not exotic; fast_array, the first test of
	// JS_DefineProperty for a property the object does not have.
	reBitTest        = `int32\(\*\(\*uint8\)\(unsafe\.Pointer\(\w+ \+ (\d+)\)\)&(0x[0-9a-f]+)>>(\d+)\) != 0`
	reExoticTest     = regexp.MustCompile(`/\* add a new property or modify an existing exotic one \*/\n\tif ` + reBitTest + ` \{\n`)
	reExtensibleTest = regexp.MustCompile(`\n\tif !!\(` + reBitTest + `\) \{\n\t\tgoto _\d+\n\t\}\n\tgoto not_extensible\n`)
	reFastArrayTest  = regexp.MustCompile(`/\* handle modification of fast array elements \*/\n\tif ` + reBitTest + ` \{\n`)
)

const (
	defineCall = "\t// Changed by internal/fixlib: a new data property of an ordinary object (see fixDefine).\n" +
		"\tif r, ok := objlitDefine(tls, %s, this_obj, prop, val, flags); ok {\n\t\treturn r\n\t}\n"
	layoutMark = "\n// Changed by internal/fixlib: the JSObject bit fields lib/objlit.go reads (see fixDefine).\n"
)

// objectBits returns the constants of the JSObject bit fields objlitDefine
// reads. They are the first five bit fields JS_NewObjectFromShape sets
// (is_std_array_prototype, extensible, free_mark, is_exotic, fast_array),
// and they must be the bits the generic path tests, in the text the
// fingerprint pins: is_exotic and extensible in JS_CreateProperty,
// fast_array in JS_DefineProperty. So a layout that changes in one place and
// not in the other stops fixDefine.
func objectBits(b []byte) ([]byte, error) {
	body, err := funcBody(b, funcHead("_JS_NewObjectFromShape"), "_JS_NewObjectFromShape")
	if err != nil {
		return nil, err
	}
	all := reObjectBits.FindAllSubmatch(body, 5)
	if len(all) != 5 {
		return nil, fmt.Errorf("JS_NewObjectFromShape changed shape: want the first five JSObject bit fields, found %d", len(all))
	}
	off := string(all[0][1])
	for i, f := range all {
		if string(f[1]) != off || !isBit(f[2], f[3]) {
			return nil, fmt.Errorf("JS_NewObjectFromShape changed shape: bit field %d is at p+%s, bit %s, mask %s", i, f[1], f[2], f[3])
		}
	}
	extensible, exotic, fastArray := string(all[1][3]), string(all[3][3]), string(all[4][3])

	create, err := funcBody(b, funcHead("_JS_CreateProperty"), "_JS_CreateProperty")
	if err != nil {
		return nil, err
	}
	define, err := funcBody(b, funcHead("XJS_DefineProperty"), "XJS_DefineProperty")
	if err != nil {
		return nil, err
	}
	for _, c := range []struct {
		field, fn, mask string
		body            []byte
		re              *regexp.Regexp
	}{
		{"is_exotic", "JS_CreateProperty", exotic, create, reExoticTest},
		{"extensible", "JS_CreateProperty", extensible, create, reExtensibleTest},
		{"fast_array", "JS_DefineProperty", fastArray, define, reFastArrayTest},
	} {
		m := findAllSubmatch(c.re, c.body)
		if len(m) != 1 {
			return nil, fmt.Errorf("%s changed shape: want one test of %s, found %d", c.fn, c.field, len(m))
		}
		if got := m[0]; string(got[1]) != off || string(got[2]) != c.mask || !isBit(got[3], got[2]) {
			return nil, fmt.Errorf("%s tests %s at +%s with mask %s, but JS_NewObjectFromShape puts it at +%s with mask %s", c.fn, c.field, got[1], got[2], off, c.mask)
		}
	}
	return []byte(fmt.Sprintf("%sconst (\n\tobjlitFlagsOff   = %s\n\tobjlitExtensible = %s\n\tobjlitExotic     = %s\n\tobjlitFastArray  = %s\n)\n",
		layoutMark, off, extensible, exotic, fastArray)), nil
}

// isBit reports whether mask is the bit shift selects.
func isBit(shift, mask []byte) bool {
	n, err := strconv.Atoi(string(shift))
	if err != nil {
		return false
	}
	m, err := strconv.ParseUint(string(mask), 0, 8)
	return err == nil && m == 1<<n
}

// defineFingerprint hashes the functions objlitDefine stands for, without
// the call fixDefine adds.
func defineFingerprint(b []byte) (string, error) {
	h := sha256.New()
	for _, name := range []string{"XJS_DefinePropertyValue", "XJS_DefineProperty", "_JS_CreateProperty", "_add_property", "_find_own_property", "_find_own_property1"} {
		text, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		if name == "XJS_DefinePropertyValue" {
			if m := reDefineValueHead.FindSubmatch(b); m != nil {
				text = bytes.Replace(text, []byte(fmt.Sprintf(defineCall, m[1])), nil, 1)
			}
		}
		h.Write(text)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// reporting reports whether fixlib prints fingerprints (-define,
// -backtrace, -regex, -lazyaddr, -inline, -poll, -runtime, -front or
// -shape) rather than checking them.
func reporting() bool {
	return *define || *backtrace || *regex || *lazyAddr || *inline || *poll || *runtimeF || *front || *shape
}

// fingerprintReport prints the map name of fingerprints fp gives for the
// generated files as they are after all of fixlib's changes, made in
// memory, so that it also works on files just taken from upstream.
func fingerprintReport(files []string, hw []*ast.File, name string, fp func([]byte) (string, error)) error {
	sort.Strings(files)
	fmt.Printf("var %s = map[string]string{\n", name)
	for _, fn := range files {
		got, err := reportedFingerprint(fn, hw, fp)
		if err != nil {
			return err
		}
		fmt.Printf("\t%q: %q,\n", filepath.Base(fn), got)
	}
	fmt.Println("}")
	return nil
}

// reportedFingerprint returns the fingerprint fp gives for the generated
// file fn after all of fixlib's changes, with the defers that free the TLS
// frames, as the checks see it.
func reportedFingerprint(fn string, hw []*ast.File, fp func([]byte) (string, error)) (string, error) {
	b, err := readEngine(fn)
	if err != nil {
		return "", err
	}
	if b, err = fixEngine(b, hw, filepath.Base(fn)); err != nil {
		return "", fmt.Errorf("%s: %v", fn, err)
	}
	if b, err = restoreDefer(b); err == nil {
		b, err = restoreLate(b)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %v", fn, err)
	}
	return fp(b)
}
