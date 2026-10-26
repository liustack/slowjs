// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// The signature LLVM binds to each indirect call. The pass gives the call a
// "kcfi" operand bundle whose type id encodes its signature code
// (-shadow-icall-sigs), llc keeps the id on the machine call, and the plugin
// (pass/icall.cpp) names a symbol at the call: goasm.icall.<function>.<n>.<id>.
// It shows as a label right before the call in llc's text and as a local
// symbol at the call's address in llc's object.
package gen

import (
	"fmt"
	"strconv"
	"strings"
)

// ICallPrefix starts the name of every indirect-call symbol.
const ICallPrefix = "goasm.icall."

// ICallUnencodable is the id the pass gives a signature code the id cannot
// hold.
const ICallUnencodable = 0xffffffff

const (
	icallParams  = "idfj"
	icallResults = "vidfj"
)

// ICallID encodes a signature code the way the pass does (icallID in
// pass/shadow.cpp): bit 31 set, the parameter count in bits 27 to 30, the
// result in bits 24 to 26 and two bits per parameter from bit 0. ok is false
// for a code it cannot hold.
func ICallID(code string) (id uint32, ok bool) {
	ps, r, found := strings.Cut(code, ">")
	if !found || len(ps) > 12 || len(r) != 1 {
		return ICallUnencodable, false
	}
	ri := strings.IndexByte(icallResults, r[0])
	if ri < 0 {
		return ICallUnencodable, false
	}
	id = 1<<31 | uint32(len(ps))<<27 | uint32(ri)<<24
	for i := 0; i < len(ps); i++ {
		p := strings.IndexByte(icallParams, ps[i])
		if p < 0 {
			return ICallUnencodable, false
		}
		id |= uint32(p) << (2 * i)
	}
	return id, true
}

// ICallCode decodes a type id into its signature code.
func ICallCode(id uint32) (string, error) {
	n := int(id >> 27 & 15)
	ri := int(id >> 24 & 7)
	if id>>31 == 0 || n > 12 || ri >= len(icallResults) || id&^(1<<31|15<<27|7<<24|(1<<(2*n)-1)) != 0 {
		return "", fmt.Errorf("type id %#x encodes no signature the generator can carry", id)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(icallParams[id>>(2*i)&3])
	}
	b.WriteByte('>')
	b.WriteByte(icallResults[ri])
	return b.String(), nil
}

// ParseICallSymbol splits an indirect-call symbol name into the function,
// the call's index in it and the type id.
func ParseICallSymbol(name string) (fn string, n int, id uint32, ok bool) {
	rest, found := strings.CutPrefix(name, ICallPrefix)
	if !found {
		return "", 0, 0, false
	}
	i := strings.LastIndexByte(rest, '.')
	if i < 0 {
		return "", 0, 0, false
	}
	v, err := strconv.ParseUint(rest[i+1:], 10, 32)
	if err != nil {
		return "", 0, 0, false
	}
	rest = rest[:i]
	j := strings.LastIndexByte(rest, '.')
	if j <= 0 {
		return "", 0, 0, false
	}
	k, err := strconv.Atoi(rest[j+1:])
	if err != nil || k < 0 {
		return "", 0, 0, false
	}
	return rest[:j], k, uint32(v), true
}
