// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"strings"
)

// GoSlot is one word of the Go argument area of a unified function: its name
// in the Go declaration of the function (GoDecl) and its offset in the area.
// Go promises only that an argument is reached by its name through the FP
// pseudo-register (name+off(FP)), not where the area sits from SP, so the
// generated assembly reads every argument and writes every result that way.
type GoSlot struct {
	Name  string
	Off   int
	Float bool // a float64, which moves through an SSE or FP register
}

// GoSlots gives the words of sig's arguments, named a0, a1 and on across the
// words (a JSValue takes two), and of its result: ret for a result of one
// word, r0 and r1 for a JSValue, none for void.
func GoSlots(sig Sig) (params, results []GoSlot) {
	argOff, retOff, _ := layout(sig)
	k := 0
	for i, p := range sig.Params {
		for w := 0; w < p.words(); w++ {
			params = append(params, GoSlot{Name: fmt.Sprintf("a%d", k), Off: argOff[i] + 8*w, Float: p == F64})
			k++
		}
	}
	switch sig.Ret {
	case Void:
	case JSV:
		results = []GoSlot{{Name: "r0", Off: retOff}, {Name: "r1", Off: retOff + 8}}
	default:
		results = []GoSlot{{Name: "ret", Off: retOff, Float: sig.Ret == F64}}
	}
	return params, results
}

// GoDecl is the Go declaration of the unified function goName of signature
// sig, with the names of GoSlots: a word of an argument is a uintptr or a
// float64, a JSValue two uintptrs, and a result of one word has no name, which
// go vet's check of the assembly calls ret.
func GoDecl(goName string, sig Sig) string {
	params, _ := GoSlots(sig)
	var ps []string
	k := 0
	for _, p := range sig.Params {
		switch p {
		case F64:
			ps = append(ps, params[k].Name+" float64")
		case JSV:
			ps = append(ps, params[k].Name+", "+params[k+1].Name+" uintptr")
		default:
			ps = append(ps, params[k].Name+" uintptr")
		}
		k += p.words()
	}
	ret := ""
	switch sig.Ret {
	case I64, Ptr:
		ret = " uintptr"
	case F64:
		ret = " float64"
	case JSV:
		ret = " (r0, r1 uintptr)"
	}
	return fmt.Sprintf("func %s(%s)%s\n", goName, strings.Join(ps, ", "), ret)
}
