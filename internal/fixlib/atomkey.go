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

// atomKeyFuncs are the functions whose behavior atomKey of lib/atomkey.go
// repeats for a key that is an atom string, and the ones it calls. They are
// hashed as ccgo writes them, before fixLazyFrames and fixInline.
var atomKeyFuncs = []string{"XJS_ValueToAtom", "XJS_ToPropertyKey", "_JS_ToStringInternal", "_JS_NewAtomStr", "_is_num_string", "_is_num", "___JS_NewAtom", "_js_get_atom_index", "___JS_AtomIsConst", "_string_get", "_js_rc"}

var (
	// The value is _val where ccgo gives JS_ValueToAtom a TLS frame (64-bit
	// targets), val where it does not.
	reAtomKeyHead = regexp.MustCompile(`\nfunc XJS_ValueToAtom\(tls \*libc\.TLS, ctx uintptr, (_?val) TJSValue\) \(r TJSAtom\) \{\n`)
	// The atom type of str, in the atom string case of __JS_NewAtom.
	reAtomKeyType = regexp.MustCompile(`\n\t\tif int32\(Tuint8_t\(\*\(\*uint32\)\(unsafe\.Pointer\(str \+ (\d+)\)\) ?& ?(0x[0-9a-f]+) ?>> ?(\d+)\)\) == atom_type \{\n\t\t\t/\* str is the atom, return its index \*/\n`)
	// The length of p, in is_num_string.
	reAtomKeyLen = regexp.MustCompile(`\n\tlen1 = int32\(\*\(\*uint32\)\(unsafe\.Pointer\(p \+ 0\)\) ?& ?(0x[0-9a-f]+) ?>> ?(\d+)\)\n\tif len1 == 0 \|\| len1 > int32\(10\) \{\n`)
	// The constants fixAtomKey adds.
	reAtomKeyConsts = regexp.MustCompile(`\nconst \(\n\tatomKeyTypeOff   = (\d+)\n\tatomKeyTypeMask  = (0x[0-9a-f]+)\n\tatomKeyTypeShift = (\d+)\n\tatomKeyLenMask   = (0x[0-9a-f]+)\n\tatomKeyLenShift  = (\d+)\n\)\n`)
)

const (
	atomKeyMark   = "\t// Changed by internal/fixlib: a string key that is an atom and no index takes atomKey of lib/atomkey.go (see fixAtomKey).\n"
	atomKeyLayout = "\n// Changed by internal/fixlib: the atom type and the length of a JSString, as lib/atomkey.go reads them (see fixAtomKey).\n"
)

// atomKeyCall is the call fixAtomKey puts at the start of JS_ValueToAtom,
// whose value is val.
func atomKeyCall(val string) string {
	return fmt.Sprintf("\tif jsvTag(%[1]s) == EJS_TAG_STRING {\n\t\tif atom, ok := atomKey(tls, ctx, jsvPtr(%[1]s)); ok {\n\t\t\treturn atom\n\t\t}\n\t}\n", val)
}

// atomKeyBits are the bit fields of a JSString that atomKey reads, as
// fixAtomKey writes them into a file.
type atomKeyBits struct {
	typeOff, typeMask, typeShift, lenMask, lenShift string
}

// fixAtomKey makes JS_ValueToAtom call atomKey of lib/atomkey.go first, for
// a key that is a string. A string that is already an atom and does not
// start with a digit (o[k] with a key the code has as a name, a key of
// for-in, Object.keys or JSON.parse) then gets its atom with no TLS frame
// and no call of JS_ToPropertyKey, JS_NewAtomStr, is_num_string and
// __JS_NewAtom. Each file gets the atom type and the length of JSString as
// its own __JS_NewAtom and is_num_string read them (s390x numbers the bit
// fields from the other end).
//
// atomKey repeats what the functions of atomKeyFuncs do for such a key, so
// their text is pinned in atomKeySites: when a release changes one of
// them, fixAtomKey stops. restoreAtomKey deletes the call and the
// constants, and fixAtomKey checks that it gives the input back.
func fixAtomKey(b []byte, file string) ([]byte, error) {
	if bytes.Contains(b, []byte(atomKeyMark)) {
		return b, nil
	}
	out, got, err := planAtomKey(b)
	if err != nil {
		return nil, err
	}
	if want := atomKeySites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: JS_ValueToAtom or a function atomKey repeats changed (fingerprint %s, want %s): read them against lib/atomkey.go, then update atomKeySites (go run ./internal/fixlib -atomkey)", file, got, want)
	}
	back, err := restoreAtomKey(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixAtomKey: restoreAtomKey does not give the input back")
	}
	return out, nil
}

// planAtomKey returns b with the change of fixAtomKey and the fingerprint
// of the functions it stands for.
func planAtomKey(b []byte) ([]byte, string, error) {
	plain, err := restoreBeforeOneFrame(bytes.Clone(b))
	if err != nil {
		return nil, "", err
	}
	got, err := funcsFingerprint(plain, atomKeyFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return nil, "", err
	}
	heads := reAtomKeyHead.FindAllSubmatchIndex(b, -1)
	if len(heads) != 1 {
		return nil, "", fmt.Errorf("want one function XJS_ValueToAtom(tls, ctx, val) (r TJSAtom), found %d", len(heads))
	}
	na, err := funcBody(b, funcHead("___JS_NewAtom"), "___JS_NewAtom")
	if err != nil {
		return nil, "", err
	}
	ty := findAllSubmatch(reAtomKeyType, na)
	if len(ty) != 1 {
		return nil, "", fmt.Errorf("__JS_NewAtom: want one test of an atom string, found %d", len(ty))
	}
	ns, err := funcBody(b, funcHead("_is_num_string"), "_is_num_string")
	if err != nil {
		return nil, "", err
	}
	ln := findAllSubmatch(reAtomKeyLen, ns)
	if len(ln) != 1 {
		return nil, "", fmt.Errorf("is_num_string: want one length of a string, found %d", len(ln))
	}
	at, val := heads[0][1], string(b[heads[0][2]:heads[0][3]])
	out := append(append(append([]byte{}, b[:at]...), atomKeyMark+atomKeyCall(val)...), b[at:]...)
	out = append(out, fmt.Sprintf("%sconst (\n\tatomKeyTypeOff   = %s\n\tatomKeyTypeMask  = %s\n\tatomKeyTypeShift = %s\n\tatomKeyLenMask   = %s\n\tatomKeyLenShift  = %s\n)\n",
		atomKeyLayout, ty[0][1], ty[0][2], ty[0][3], ln[0][1], ln[0][2])...)
	return out, got, nil
}

// restoreAtomKey undoes fixAtomKey.
func restoreAtomKey(b []byte) ([]byte, error) {
	if i := bytes.Index(b, []byte(atomKeyLayout)); i >= 0 {
		end := bytes.Index(b[i:], []byte("\n)\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of the constants of fixAtomKey not found")
		}
		b = append(b[:i:i], b[i+end+3:]...)
	}
	out := b
	for _, val := range []string{"_val", "val"} {
		out = bytes.Replace(out, []byte(atomKeyMark+atomKeyCall(val)), nil, 1)
	}
	if bytes.Contains(out, []byte("atomKey(tls, ")) {
		return nil, fmt.Errorf("restoreAtomKey: a call of atomKey is left")
	}
	return out, nil
}

// atomKeyConsts returns the bit fields fixAtomKey wrote into b.
func atomKeyConsts(b []byte) (atomKeyBits, error) {
	i := bytes.Index(b, []byte(atomKeyLayout))
	if i < 0 {
		return atomKeyBits{}, fmt.Errorf("no constants of fixAtomKey")
	}
	m := reAtomKeyConsts.FindSubmatch(b[i+len(atomKeyLayout)-1:])
	if m == nil {
		return atomKeyBits{}, fmt.Errorf("the constants of fixAtomKey changed shape")
	}
	return atomKeyBits{string(m[1]), string(m[2]), string(m[3]), string(m[4]), string(m[5])}, nil
}

// atomKeyFingerprint is the fingerprint fixAtomKey checks, for -atomkey.
func atomKeyFingerprint(b []byte) (string, error) {
	back, err := restoreAtomKey(b)
	if err != nil {
		return "", err
	}
	_, fp, err := planAtomKey(back)
	return fp, err
}
