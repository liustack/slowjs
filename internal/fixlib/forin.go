// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fixForIn makes js_for_in_prepare_prototype_chain_enum ask forInNoEnum of
// lib/forin.go first whether a prototype has any enumerable property. When a
// for in loop runs out of own properties, the engine checks each object of
// the prototype chain for enumerable properties, to know whether it must
// walk the chain. It lists the enumerable property names of each one with
// JS_GetOwnPropertyNamesInternal: a malloc, a duplicated atom per name and
// a free, in every loop, mostly to find that Object.prototype has none.
// forInNoEnum reads that from the shape of an object that is not exotic and
// gives no names, as JS_GetOwnPropertyNamesInternal does, so the loop goes
// on as before. For an exotic object, or one with an enumerable property,
// the engine lists the names as before.
//
// forInNoEnum repeats what JS_GetOwnPropertyNamesInternal counts, and reads
// two bit fields: the flags of a shape property and the is_exotic bit of an
// object. Each file gets them as constants, taken from the tests in its own
// JS_GetOwnPropertyNamesInternal. The text of
// js_for_in_prepare_prototype_chain_enum, of
// JS_GetOwnPropertyNamesInternal, of get_shape_prop and of
// JS_FreePropertyEnum, which frees the empty list, is pinned in forInSites:
// when a release changes one of them, fixForIn stops, and the new code must
// be read against forInNoEnum before the fingerprints are updated (go run
// ./internal/fixlib -forin prints them). The fingerprints are of the text
// without the change, after the changes before it.
func fixForIn(b []byte, file string) ([]byte, error) {
	got, err := forInFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := forInSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: js_for_in_prepare_prototype_chain_enum, JS_GetOwnPropertyNamesInternal, get_shape_prop or JS_FreePropertyEnum changed (fingerprint %s, want %s): read them against lib/forin.go, then update forInSites (go run ./internal/fixlib -forin)", file, got, want)
	}

	body, err := funcBody(b, funcHead(forInFunc), forInFunc)
	if err != nil {
		return nil, err
	}
	start := bytes.Index(b, body)
	fixed, err := forInCallFirst(body)
	if err != nil {
		return nil, err
	}
	b = append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...)

	layout, err := forInBits(b)
	if err != nil {
		return nil, err
	}
	return putConstants(b, forInLayoutMark, layout)
}

const (
	forInFunc = "_js_for_in_prepare_prototype_chain_enum"
	// The flags of the names of a prototype: JS_GPN_STRING_MASK and
	// JS_GPN_ENUM_ONLY.
	forInFlags      = `libc.Int32FromInt32(1)<<libc.Int32FromInt32(0)|libc.Int32FromInt32(1)<<libc.Int32FromInt32(4)`
	forInComment    = "// Changed by internal/fixlib: no names to list for a prototype without enumerable properties (see fixForIn).\n"
	forInLayoutMark = "\n// Changed by internal/fixlib: the bit fields lib/forin.go reads (see fixForIn).\n"
)

var (
	// The call that lists the enumerable names of a prototype, as ccgo
	// writes it, and as fixForIn leaves it: the context, the two out
	// parameters and the object.
	reForInNames      = regexp.MustCompile(`(\t+)if _JS_GetOwnPropertyNamesInternal\(tls, (ctx\d*), ([^,\n]+), ([^,\n]+), ([^,\n]+), ` + regexp.QuoteMeta(forInFlags) + `\) != 0 \{\n`)
	reForInNamesFixed = regexp.MustCompile(`(\t+)` + regexp.QuoteMeta(forInComment) + `(\t+)if !forInNoEnum\(tls, ([^,\n]+), ([^,\n]+), ([^,\n]+)\) && _JS_GetOwnPropertyNamesInternal\(tls, (ctx\d*), ([^,\n]+), ([^,\n]+), ([^,\n]+), ` + regexp.QuoteMeta(forInFlags) + `\) != 0 \{\n`)

	// The enumerable flag of a shape property, read as
	// JS_GetOwnPropertyNamesInternal reads it in both of its loops, and its
	// first test of is_exotic, which counts the elements of a fast array.
	reForInEnumerable = regexp.MustCompile(`is_enumerable = libc\.BoolInt32\(int32\(\*\(\*uint32\)\(unsafe\.Pointer\(prs \+ 0\)\)&(0x[0-9a-f]+)>>(\d+)\)&\(libc\.Int32FromInt32\(1\)<<libc\.Int32FromInt32\(2\)\) != libc\.Int32FromInt32\(0\)\)\n`)
	reForInExotic     = regexp.MustCompile(`\n\tif int32\(\*\(\*uint8\)\(unsafe\.Pointer\(p \+ (\d+)\)\)&(0x[0-9a-f]+)>>(\d+)\) != 0 \{\n\t\tif int32\(\*\(\*uint8\)\(unsafe\.Pointer\(p \+ \d+\)\)&0x[0-9a-f]+>>\d+\) != 0 \{\n\t\t\tif flags&\(libc\.Int32FromInt32\(1\)<<libc\.Int32FromInt32\(0\)\) != 0 \{\n\t\t\t\tnum_keys_count = `)
)

// forInCallFirst puts the call of forInNoEnum before the listing of the
// names of a prototype in body, the text of
// js_for_in_prepare_prototype_chain_enum, or checks that it is there.
func forInCallFirst(body []byte) ([]byte, error) {
	old, fixed := findAllSubmatch(reForInNames, body), findAllSubmatch(reForInNamesFixed, body)
	switch {
	case len(old) == 1 && len(fixed) == 0:
		m := old[0]
		indent, obj, ptab, plen := m[1], m[5], m[3], m[4]
		line := reForInNames.Find(body)
		text := fmt.Sprintf("%s%s%sif !forInNoEnum(tls, %s, %s, %s) && %s", indent, forInComment, indent, obj, ptab, plen, bytes.TrimPrefix(line, append(indent, "if "...)))
		return bytes.Replace(body, line, []byte(text), 1), nil
	case len(old) == 0 && len(fixed) == 1:
		m := fixed[0]
		if !bytes.Equal(m[1], m[2]) || !bytes.Equal(m[3], m[9]) || !bytes.Equal(m[4], m[7]) || !bytes.Equal(m[5], m[8]) {
			return nil, fmt.Errorf("%s: forInNoEnum does not get the arguments of JS_GetOwnPropertyNamesInternal", forInFunc)
		}
		return body, nil
	}
	return nil, fmt.Errorf("%s: want one listing of the enumerable names of a prototype, found %d, and %d after forInNoEnum", forInFunc, len(old), len(fixed))
}

// unfixForIn gives body, the text of
// js_for_in_prepare_prototype_chain_enum, without the call of forInNoEnum.
func unfixForIn(body []byte) []byte {
	return reForInNamesFixed.ReplaceAll(body, []byte("${1}if _JS_GetOwnPropertyNamesInternal(tls, ${6}, ${7}, ${8}, ${9}, "+forInFlags+") != 0 {\n"))
}

// forInBits returns the constants of the bit fields forInNoEnum reads, from
// the tests of JS_GetOwnPropertyNamesInternal: both of its reads of the
// enumerable flag of a shape property must agree.
func forInBits(b []byte) ([]byte, error) {
	body, err := funcBody(b, funcHead("_JS_GetOwnPropertyNamesInternal"), "_JS_GetOwnPropertyNamesInternal")
	if err != nil {
		return nil, err
	}
	enum := findAllSubmatch(reForInEnumerable, body)
	if len(enum) != 2 {
		return nil, fmt.Errorf("JS_GetOwnPropertyNamesInternal changed shape: want 2 reads of the enumerable flag, found %d", len(enum))
	}
	if !bytes.Equal(enum[0][1], enum[1][1]) || !bytes.Equal(enum[0][2], enum[1][2]) {
		return nil, fmt.Errorf("JS_GetOwnPropertyNamesInternal reads the flags of a shape property with %s>>%s and with %s>>%s", enum[0][1], enum[0][2], enum[1][1], enum[1][2])
	}
	exotic := findAllSubmatch(reForInExotic, body)
	if len(exotic) != 1 {
		return nil, fmt.Errorf("JS_GetOwnPropertyNamesInternal changed shape: want 1 test of is_exotic before the count of a fast array, found %d", len(exotic))
	}
	if !isBit(exotic[0][3], exotic[0][2]) {
		return nil, fmt.Errorf("JS_GetOwnPropertyNamesInternal tests is_exotic with mask %s and shift %s", exotic[0][2], exotic[0][3])
	}
	return []byte(fmt.Sprintf("%sconst (\n\tforInFlagsMask  = %s\n\tforInFlagsShift = %s\n\tforInExoticOff  = %s\n\tforInExotic     = %s\n)\n",
		forInLayoutMark, enum[0][1], enum[0][2], exotic[0][1], exotic[0][2])), nil
}

// forInFingerprint hashes the functions forInNoEnum stands for and is
// called from, without the change of fixForIn.
func forInFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, []string{forInFunc, "_JS_GetOwnPropertyNamesInternal", "_get_shape_prop", "XJS_FreePropertyEnum"}, func(text []byte) []byte {
		return unfixForIn(text)
	})
}
