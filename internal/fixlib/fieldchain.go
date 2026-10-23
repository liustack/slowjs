// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fieldChainFuncs are the functions whose answer fieldProto and fieldGoesOn
// of lib/fieldchain.go repeat: JS_GetPropertyInternal for a string, a
// number and a fast array, the prototype JS_GetPrototypePrimitive gives a
// primitive, and the test of an index atom. They are hashed as ccgo writes
// them, before fixLazyFrames and fixInline.
var fieldChainFuncs = []string{"XJS_GetPropertyInternal", "_JS_GetPrototypePrimitive", "___JS_AtomIsTaggedInt"}

// fieldChainOps are the handlers fixFieldChain changes, named by the label
// of their slow path. get_length is left as it is: length is a key a string
// has of its own, and every array has.
var fieldChainOps = []string{"get_field", "get_field2"}

const (
	fieldProtoMark  = "// Changed by internal/fixlib: a string or a number looks for the key from its prototype (see fixFieldChain).\n"
	fieldGoesOnMark = "// Changed by internal/fixlib: a fast array goes on to its prototype for a key that is no index (see fixFieldChain).\n"
	fieldLayout     = "\n// Changed by internal/fixlib: the fast_array bit of a JSObject, as lib/fieldchain.go reads it (see fixFieldChain).\n"
)

var (
	// The start of a handler of JS_CallInternal that reads a property by
	// name, as ccgo writes it (fixExpect comes later): the test of an
	// object, then the loop over the object and its prototypes. 1: BoolInt64
	// or BoolInt32, 2: the offset of the value in the
	// frame, 3: the label that goes to the slow path, 4: the object, 5: the
	// property found, 6: the offset of its address, 7: the atom.
	reFieldEntry = regexp.MustCompile(`\n\t\tif !\(libc\.(BoolInt(?:64|32))\(!!\(int32\(\(\*\(\*TJSValue\)\(unsafe\.Pointer\(bp \+ (\d+)\)\)\)\.Ftag\) == int32\(EJS_TAG_OBJECT\)\)\) != 0\) \{\n\t\t\tgoto (_\d+)\n\t\t\}\n\t\t(p\d+) = \*\(\*uintptr\)\(unsafe\.Pointer\(bp \+ (?:\d+)\)\)\n\t\tfor \{\n\t\t\t(prs\d*) = _find_own_property\(tls, bp\+(\d+), (?:p\d+), (atom\d+)\)\n`)
	// The test of exotic behavior of the object of the loop, where the
	// handler leaves for the slow path. 1: the indent and the test, 2: the
	// object.
	reFieldExotic = regexp.MustCompile(`\n(\t\t\tif libc\.BoolInt(?:64|32)\(!!\(int32\(\*\(\*uint8\)\(unsafe\.Pointer\((p\d+) \+ \d+\)\)&0x[0-9a-f]+>>\d+\) != 0\)\) != 0)( \{ /\* XXX: should avoid the slow path for arrays)`)
	// The slow path of a handler: 1: its label, 2: its name, 3: the context.
	reFieldSlow = regexp.MustCompile(`\n\t(_\d+):\n\t\t;\n\t\tgoto (\w+)_slow_path\n\t(?:\w+)_slow_path:\n\t\t;\n\t\t\(\*TJSStackFrame\)\(unsafe\.Pointer\(sf\)\)\.Fcur_pc = pc\n\t\tval\d+ = XJS_GetPropertyInternal\(tls, (ctx\d*), `)
	// The fast_array test right after the test of exotic behavior in
	// JS_GetPropertyInternal: 1: the offset, 2: the mask.
	reFieldFastArray = regexp.MustCompile(`\n\t\t\t/\* exotic behaviors \*/\n\t\t\tif int32\(\*\(\*uint8\)\(unsafe\.Pointer\(p2 \+ (\d+)\)\)&(0x[0-9a-f]+)>>\d+\) != 0 \{\n\t\t\t\tif ___JS_AtomIsTaggedInt\(tls, prop\) != 0 \{\n`)
	reFieldConsts    = regexp.MustCompile(`\nconst \(\n\tfieldFastArrayOff  = (\d+)\n\tfieldFastArrayMask = (0x[0-9a-f]+)\n\)\n`)
	reFieldGoesOn    = regexp.MustCompile(`\t+` + regexp.QuoteMeta(fieldGoesOnMark) + `(\t+if [^\n]*) && !fieldGoesOn\(tls, p\d+, atom\d+\)( \{ /\* XXX)`)
	reFieldProto     = regexp.MustCompile(`\n\t\tif !\(libc\.(BoolInt(?:64|32))\(!!\(int32\(\(\*\(\*TJSValue\)\(unsafe\.Pointer\(bp \+ (\d+)\)\)\)\.Ftag\) == int32\(EJS_TAG_OBJECT\)\)\) != 0\) \{\n\t\t\t` + regexp.QuoteMeta(fieldProtoMark) + `\t\t\t(p\d+) = fieldProto\(tls, ctx\d*, \*\(\*TJSValue\)\(unsafe\.Pointer\(bp \+ \d+\)\), atom\d+\)\n\t\t\tif p\d+ == 0 \{\n\t\t\t\tgoto (_\d+)\n\t\t\t\}\n\t\t\} else \{\n\t\t\tp\d+ = \*\(\*uintptr\)\(unsafe\.Pointer\(bp \+ \d+\)\)\n\t\t\}\n`)
)

// fixFieldChain makes get_field and get_field2 of JS_CallInternal keep
// looking for a key where JS_GetPropertyInternal, their slow path, would
// look anyway (see lib/fieldchain.go):
//
//   - a value that is not an object starts the loop over prototypes at the
//     prototype fieldProto gives, and takes the slow path when it gives 0
//   - an object with exotic behavior that fieldGoesOn says goes on to its
//     prototype does not leave the loop for the slow path
//
// The value stays on the stack, and the slow path still gets it as this
// for a getter or an exotic object further up the chain. Only the
// files of 64-bit targets (cellFiles) change: fieldProto reads the struct
// JSValue of those. Each file gets the fast_array bit as its own
// JS_GetPropertyInternal reads it. The functions fieldProto and fieldGoesOn
// stand for are pinned in fieldChainSites, and the handlers must read as
// reFieldEntry, reFieldExotic and reFieldSlow say, once each.
// restoreFieldChain undoes the change, and fixFieldChain checks that it
// gives the input back.
func fixFieldChain(b []byte, file string) ([]byte, error) {
	if bytes.Contains(b, []byte(fieldProtoMark)) {
		return b, nil
	}
	out, got, err := planFieldChain(b, cellFiles[file])
	if err != nil {
		return nil, err
	}
	if want := fieldChainSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: JS_GetPropertyInternal, JS_GetPrototypePrimitive or __JS_AtomIsTaggedInt changed (fingerprint %s, want %s): read them against lib/fieldchain.go, then update fieldChainSites (go run ./internal/fixlib -fieldchain)", file, got, want)
	}
	back, err := restoreFieldChain(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixFieldChain: restoreFieldChain does not give the input back")
	}
	return out, nil
}

// planFieldChain returns b with the change of fixFieldChain when change is
// set (b as it is otherwise), and the fingerprint of the functions it stands
// for.
func planFieldChain(b []byte, change bool) ([]byte, string, error) {
	plain, err := restoreBeforeOneFrame(bytes.Clone(b))
	if err != nil {
		return nil, "", err
	}
	got, err := funcsFingerprint(plain, fieldChainFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return nil, "", err
	}
	if !change {
		return b, got, nil
	}
	gp, err := funcBody(b, funcHead("XJS_GetPropertyInternal"), "XJS_GetPropertyInternal")
	if err != nil {
		return nil, "", err
	}
	fa := findAllSubmatch(reFieldFastArray, gp)
	if len(fa) != 1 {
		return nil, "", fmt.Errorf("JS_GetPropertyInternal: want one test of a fast array after the test of exotic behavior, found %d", len(fa))
	}
	head := funcHead("_JS_CallInternal")
	body, err := funcBody(b, head, "_JS_CallInternal")
	if err != nil {
		return nil, "", err
	}
	start := bytes.Index(b, body)
	slows := map[string][][]byte{}
	for _, m := range findAllSubmatch(reFieldSlow, body) {
		slows[string(m[2])] = m
	}
	entries := reFieldEntry.FindAllSubmatchIndex(body, -1)
	exotics := reFieldExotic.FindAllSubmatchIndex(body, -1)
	type edit struct {
		at, end int
		text    string
	}
	var edits []edit
	for _, op := range fieldChainOps {
		slow := slows[op]
		if slow == nil {
			return nil, "", fmt.Errorf("_JS_CallInternal: no slow path of %s that calls JS_GetPropertyInternal", op)
		}
		label, ctx := string(slow[1]), string(slow[3])
		var entry []int
		for _, e := range entries {
			if string(body[e[6]:e[7]]) == label {
				if entry != nil {
					return nil, "", fmt.Errorf("_JS_CallInternal: two loops of %s", op)
				}
				entry = e
			}
		}
		if entry == nil {
			return nil, "", fmt.Errorf("_JS_CallInternal: the loop of %s over prototypes changed shape", op)
		}
		boolFn, off, obj, atom := string(body[entry[2]:entry[3]]), string(body[entry[4]:entry[5]]), string(body[entry[8]:entry[9]]), string(body[entry[14]:entry[15]])
		var exotic []int
		for _, x := range exotics {
			if x[0] > entry[1] && string(body[x[4]:x[5]]) == obj && (exotic == nil || x[0] < exotic[0]) {
				exotic = x
			}
		}
		if exotic == nil || bytes.Contains(body[entry[1]:exotic[0]], []byte("\n\t\tfor {\n")) {
			return nil, "", fmt.Errorf("_JS_CallInternal: no test of exotic behavior in the loop of %s", op)
		}
		prs, at := string(body[entry[10]:entry[11]]), string(body[entry[12]:entry[13]])
		edits = append(edits, edit{entry[0], entry[1], fmt.Sprintf("\n\t\tif !(libc.%[1]s(!!(int32((*(*TJSValue)(unsafe.Pointer(bp + %[2]s))).Ftag) == int32(EJS_TAG_OBJECT))) != 0) {\n\t\t\t%[7]s\t\t\t%[3]s = fieldProto(tls, %[8]s, *(*TJSValue)(unsafe.Pointer(bp + %[2]s)), %[6]s)\n\t\t\tif %[3]s == 0 {\n\t\t\t\tgoto %[9]s\n\t\t\t}\n\t\t} else {\n\t\t\t%[3]s = *(*uintptr)(unsafe.Pointer(bp + %[2]s))\n\t\t}\n\t\tfor {\n\t\t\t%[4]s = _find_own_property(tls, bp+%[5]s, %[3]s, %[6]s)\n",
			boolFn, off, obj, prs, at, atom, fieldProtoMark, ctx, label)})
		edits = append(edits, edit{exotic[0], exotic[1], fmt.Sprintf("\n\t\t\t%s%s && !fieldGoesOn(tls, %s, %s)%s", fieldGoesOnMark, body[exotic[2]:exotic[3]], obj, atom, body[exotic[6]:exotic[7]])})
	}
	for i := 0; i < len(edits); i++ {
		for j := i + 1; j < len(edits); j++ {
			if edits[j].at < edits[i].at {
				edits[i], edits[j] = edits[j], edits[i]
			}
		}
	}
	var nb []byte
	pos := 0
	for _, e := range edits {
		nb = append(append(nb, body[pos:e.at]...), e.text...)
		pos = e.end
	}
	nb = append(nb, body[pos:]...)
	out := append(append(append([]byte{}, b[:start]...), nb...), b[start+len(body):]...)
	out = append(out, fmt.Sprintf("%sconst (\n\tfieldFastArrayOff  = %s\n\tfieldFastArrayMask = %s\n)\n", fieldLayout, fa[0][1], fa[0][2])...)
	return out, got, nil
}

// restoreFieldChain undoes fixFieldChain.
func restoreFieldChain(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte(fieldLayout)); i >= 0 {
		end := bytes.Index(b[i:], []byte("\n)\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of the constants of fixFieldChain not found")
		}
		b = append(b[:i:i], b[i+end+3:]...)
	}
	out := reFieldGoesOn.ReplaceAll(b, []byte("$1$2"))
	out = reFieldProto.ReplaceAllFunc(out, func(m []byte) []byte {
		s := reFieldProto.FindSubmatch(m)
		obj, off := string(s[3]), string(s[2])
		return []byte(fmt.Sprintf("\n\t\tif !(libc.%s(!!(int32((*(*TJSValue)(unsafe.Pointer(bp + %s))).Ftag) == int32(EJS_TAG_OBJECT))) != 0) {\n\t\t\tgoto %s\n\t\t}\n\t\t%s = *(*uintptr)(unsafe.Pointer(bp + %s))\n", s[1], off, s[4], obj, off))
	})
	if bytes.Contains(out, []byte("fieldProto(tls, ")) || bytes.Contains(out, []byte("fieldGoesOn(tls, ")) {
		return nil, fmt.Errorf("restoreFieldChain: a call of fieldProto or fieldGoesOn is left")
	}
	return out, nil
}

// fieldChainConsts returns the fast_array bit fixFieldChain wrote into b.
func fieldChainConsts(b []byte) (off, mask string, err error) {
	i := bytes.Index(b, []byte(fieldLayout))
	if i < 0 {
		return "", "", fmt.Errorf("no constants of fixFieldChain")
	}
	m := reFieldConsts.FindSubmatch(b[i+len(fieldLayout)-1:])
	if m == nil {
		return "", "", fmt.Errorf("the constants of fixFieldChain changed shape")
	}
	return string(m[1]), string(m[2]), nil
}

// fieldChainFingerprint is the fingerprint fixFieldChain checks, for
// -fieldchain.
func fieldChainFingerprint(b []byte) (string, error) {
	back, err := restoreFieldChain(b)
	if err != nil {
		return "", err
	}
	_, fp, err := planFieldChain(back, false)
	return fp, err
}
