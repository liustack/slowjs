// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// jsonFuncs are the functions whose behavior jsonQuoteRun and
// jsonStringRun of lib/jsonstr.go repeat for a run of characters, and the
// ones they call, without the change of fixJSONStrings.
var jsonFuncs = []string{"_JS_ToQuotedString", "_string_getc", "_string_get", "_json_parse_string", "Xunicode_from_utf8",
	"_string_buffer_write8", "_string_buffer_write16", "_string_buffer_realloc", "_string_buffer_widen",
	"_string_buffer_putc", "_string_buffer_putc_slow", "_string_buffer_putc8", "_string_buffer_putc16", "_string_buffer_putc16_slow"}

var (
	// The loop of JS_ToQuotedString over the characters of p1, as far as
	// the call of string_getc: the index (twice, once without spaces) and
	// the length of p1.
	reQuoteLoop = regexp.MustCompile(`\n\tfor \{\n(\t\tif !\(\*\(\*int32\)\(unsafe\.Pointer\((bp(?: \+ \d+)?)\)\) < int32\(\*\(\*uint32\)\(unsafe\.Pointer\(p1 \+ (\d+)\)\) ?& ?(0x[0-9a-f]+) ?>> ?(\d+)\)\) \{\n\t\t\tbreak\n\t\t\}\n\t\tc1 = (?:libc\.Uint32FromInt32|uint32)\(_string_getc\(tls, p1, (bp(?:\+\d+)?)\)\)\n)`)
	// The start of the loop of json_parse_string.
	reParseLoop = regexp.MustCompile(`\n\tp = \*\(\*uintptr\)\(unsafe\.Pointer\(pp\)\)\n\tfor \{\n(\t\tif p >= \(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fbuf_end \{\n\t\t\tgoto end_of_input\n\t\t\}\n\t\tv2 = p\n\t\tp = p \+ 1\n\t\tc1 = uint32\(\*\(\*Tuint8_t\)\(unsafe\.Pointer\(v2\)\)\)\n\t\tif c1 == (?:libc\.Uint32FromInt32|uint32)\(sep\) \{\n)`)
	// The 16-bit flag of p and the offset of its characters, in
	// string_get.
	reJSONWide = regexp.MustCompile(`\n\tif int32\(Tuint8_t\(\*\(\*uint32\)\(unsafe\.Pointer\(p \+ (\d+)\)\) ?& ?(0x[0-9a-f]+) ?>> ?(\d+)\)\) != 0 \{\n\t\tv1 = (?:libc\.Int32FromUint16|int32)\(\*\(\*Tuint16_t\)\(unsafe\.Pointer\(p \+ (\d+) \+ uintptr\(idx\)\*2\)\)\)\n\t\} else \{\n\t\tv1 = (?:libc\.Int32FromUint8|int32)\(\*\(\*Tuint8_t\)\(unsafe\.Pointer\(p \+ (\d+) \+ uintptr\(idx\)\)\)\)\n\t\}\n`)
)

const (
	jsonQuoteMark = "\t\t// Changed by internal/fixlib: copy the characters JSON.stringify keeps as they are in one run (see fixJSONStrings).\n"
	jsonParseMark = "\t\t// Changed by internal/fixlib: copy the characters JSON.parse takes as they are in one run (see fixJSONStrings).\n"
	jsonParseCall = jsonParseMark + "\t\tif p = jsonStringRun(tls, b, p, (*TJSParseState)(unsafe.Pointer(s)).Fbuf_end, sep); p == 0 {\n\t\t\tgoto fail\n\t\t}\n"
	jsonLayout    = "\n// Changed by internal/fixlib: the length, the 16-bit flag and the characters of a JSString, as lib/jsonstr.go reads them (see fixJSONStrings).\n"
)

// jsonQuoteCall is the call fixJSONStrings puts at the start of the loop of
// JS_ToQuotedString, whose index is at i.
func jsonQuoteCall(i string) string {
	return jsonQuoteMark + "\t\tif jsonQuoteRun(tls, b, p1, " + i + ") != 0 {\n\t\t\tgoto fail\n\t\t}\n"
}

// fixJSONStrings makes the loops over the characters of a string in
// JSON.stringify (JS_ToQuotedString) and JSON.parse (json_parse_string)
// call jsonQuoteRun and jsonStringRun of lib/jsonstr.go at the start of
// each turn. They copy the run of characters from there that the loop
// would put into the StringBuffer one by one, in a few calls, and leave
// the loop the character that ended the run: one to escape, the end of the
// string, or an error. Each file gets the length, the 16-bit flag and the
// offset of the characters of a JSString as its own JS_ToQuotedString and
// string_get read them (s390x numbers the bit fields from the other end).
//
// The two functions repeat what the loops and the functions of jsonFuncs
// do for such a run, so their text is pinned in jsonSites: when a release
// changes one of them, fixJSONStrings stops. It starts from the text
// without its change (restoreJSONStrings), and checks that
// restoreJSONStrings gives that text back.
func fixJSONStrings(b []byte, file string) ([]byte, error) {
	plain, err := restoreJSONStrings(b)
	if err != nil {
		return nil, err
	}
	out, got, err := planJSONStrings(plain)
	if err != nil {
		return nil, err
	}
	if want := jsonSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/jsonstr.go, then update jsonSites (go run ./internal/fixlib -json)", file, strings.Join(jsonFuncs, ", "), got, want)
	}
	back, err := restoreJSONStrings(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, plain) {
		return nil, fmt.Errorf("fixJSONStrings: restoreJSONStrings does not give the input back")
	}
	return out, nil
}

// planJSONStrings returns b, which has no change of fixJSONStrings, with
// the change, and the fingerprint of jsonFuncs.
func planJSONStrings(b []byte) ([]byte, string, error) {
	got, err := funcsFingerprint(b, jsonFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return nil, "", err
	}
	out, n, err := jsonInsert(b, "_JS_ToQuotedString", reQuoteLoop, func(m [][]byte) (string, error) {
		if i := strings.ReplaceAll(string(m[2]), " ", ""); i != string(m[6]) {
			return "", fmt.Errorf("_JS_ToQuotedString: the loop tests the index at %s and string_getc moves it at %s", m[2], m[6])
		}
		return jsonQuoteCall(string(m[6])), nil
	})
	if err != nil {
		return nil, "", err
	}
	if out, _, err = jsonInsert(out, "_json_parse_string", reParseLoop, func([][]byte) (string, error) { return jsonParseCall, nil }); err != nil {
		return nil, "", err
	}
	sg, err := funcBody(b, funcHead("_string_get"), "_string_get")
	if err != nil {
		return nil, "", err
	}
	w := findAllSubmatch(reJSONWide, sg)
	if len(w) != 1 {
		return nil, "", fmt.Errorf("string_get: want one read of a character of 16 or 8 bits, found %d", len(w))
	}
	if string(w[0][4]) != string(w[0][5]) {
		return nil, "", fmt.Errorf("string_get: the characters of 16 bits start at %s, of 8 bits at %s", w[0][4], w[0][5])
	}
	// The constants go right after string_get, not at the end of the
	// file, where other changes add theirs.
	consts := fmt.Sprintf("%sconst (\n\tjsonStrLenOff    = %s\n\tjsonStrLenMask   = %s\n\tjsonStrLenShift  = %s\n\tjsonStrWideOff   = %s\n\tjsonStrWideMask  = %s\n\tjsonStrWideShift = %s\n\tjsonStrData      = %s\n)\n",
		jsonLayout, n[3], n[4], n[5], w[0][1], w[0][2], w[0][3], w[0][4])
	if sg, err = funcBody(out, funcHead("_string_get"), "_string_get"); err != nil {
		return nil, "", err
	}
	at := bytes.Index(out, sg) + len(sg)
	return append(append(append([]byte{}, out[:at]...), consts...), out[at:]...), got, nil
}

// jsonInsert puts the text call gives for the match of re into the
// function name of b, before the first group of the match, which must be
// the only one in the function. It returns the match too.
func jsonInsert(b []byte, name string, re *regexp.Regexp, call func([][]byte) (string, error)) ([]byte, [][]byte, error) {
	body, err := funcBody(b, funcHead(name), name)
	if err != nil {
		return nil, nil, err
	}
	loc := re.FindAllSubmatchIndex(body, -1)
	if len(loc) != 1 {
		return nil, nil, fmt.Errorf("%s: want one loop over the characters as fixJSONStrings reads it, found %d: read it against lib/jsonstr.go", name, len(loc))
	}
	m := make([][]byte, len(loc[0])/2)
	for i := range m {
		m[i] = body[loc[0][2*i]:loc[0][2*i+1]]
	}
	text, err := call(m)
	if err != nil {
		return nil, nil, err
	}
	start := bytes.Index(b, body)
	at := start + loc[0][2]
	return append(append(append([]byte{}, b[:at]...), text...), b[at:]...), m, nil
}

// restoreJSONStrings undoes fixJSONStrings.
func restoreJSONStrings(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte(jsonLayout)); i >= 0 {
		end := bytes.Index(b[i:], []byte("\n)\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of the constants of fixJSONStrings not found")
		}
		b = append(b[:i:i], b[i+end+3:]...)
	}
	out := bytes.Replace(b, []byte(jsonParseCall), nil, 1)
	if i := bytes.Index(out, []byte(jsonQuoteMark)); i >= 0 {
		m := reJSONQuoteCall.FindIndex(out[i:])
		if m == nil || m[0] != 0 {
			return nil, fmt.Errorf("restoreJSONStrings: the call of jsonQuoteRun is not what fixJSONStrings writes")
		}
		out = append(out[:i:i], out[i+m[1]:]...)
	}
	if bytes.Contains(out, []byte("jsonQuoteRun(tls")) || bytes.Contains(out, []byte("jsonStringRun(tls")) {
		return nil, fmt.Errorf("restoreJSONStrings: a call of jsonQuoteRun or jsonStringRun is left")
	}
	return out, nil
}

// The call of jsonQuoteCall, from its mark.
var reJSONQuoteCall = regexp.MustCompile(regexp.QuoteMeta(jsonQuoteMark) + `\t\tif jsonQuoteRun\(tls, b, p1, bp(?:\+\d+)?\) != 0 \{\n\t\t\tgoto fail\n\t\t\}\n`)

// jsonFingerprint is the fingerprint fixJSONStrings checks, for -json.
func jsonFingerprint(b []byte) (string, error) {
	plain, err := restoreJSONStrings(b)
	if err != nil {
		return "", err
	}
	_, fp, err := planJSONStrings(plain)
	return fp, err
}
