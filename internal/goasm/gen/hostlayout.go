// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"strconv"
	"strings"
)

// HostLayoutSymbol is the table of int64 that engine/quickjs.c defines with
// the layout of values a host reads and writes in place (JS_HOST_LAYOUT_* in
// engine/quickjs.h).
const HostLayoutSymbol = "js_host_layout"

// HostLayoutNames are the Go names of the entries of the table, in the order
// of JS_HOST_LAYOUT_* in engine/quickjs.h.
var HostLayoutNames = []string{
	"layoutRefCountOffset",
	"layoutStringLenMask",
	"layoutStringWideBit",
	"layoutStringChars",
	"layoutStringLenMax",
	"atomEmptyString",
	"atomFileName",
	"atomLineNumber",
	"atomColumnNumber",
	"atomMessage",
	"atomStack",
	"atomName",
	"atomToString",
	"atomThen",
}

// HostLayout reads the table HostLayoutSymbol from llc's text: one value per
// name of HostLayoutNames, from .quad lines and .zero runs of whole quads.
func HostLayout(u *AsmUnit) ([]int64, error) {
	o := u.Data[HostLayoutSymbol]
	if o == nil {
		return nil, fmt.Errorf("no %s in the engine", HostLayoutSymbol)
	}
	var vs []int64
	for _, l := range o.Lines {
		dir, arg, _ := strings.Cut(strings.Join(strings.Fields(l), " "), " ")
		switch dir {
		case ".quad", ".xword":
			v, err := strconv.ParseInt(arg, 0, 64)
			if err != nil {
				u, uerr := strconv.ParseUint(arg, 0, 64)
				if uerr != nil {
					return nil, fmt.Errorf("%s: %q is not a number", HostLayoutSymbol, l)
				}
				v = int64(u)
			}
			vs = append(vs, v)
		case ".zero":
			n, err := strconv.Atoi(arg)
			if err != nil || n%8 != 0 {
				return nil, fmt.Errorf("%s: %q is not a run of whole quads", HostLayoutSymbol, l)
			}
			for range n / 8 {
				vs = append(vs, 0)
			}
		default:
			return nil, fmt.Errorf("%s: %q is not a quad", HostLayoutSymbol, l)
		}
	}
	if len(vs) != len(HostLayoutNames) {
		return nil, fmt.Errorf("%s has %d entries, the generator names %d (HostLayoutNames)", HostLayoutSymbol, len(vs), len(HostLayoutNames))
	}
	return vs, nil
}

// HostLayoutDecls writes the values of HostLayout as Go constants, nothing
// for none.
func HostLayoutDecls(vs []int64) string {
	if vs == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("// The layout of values the host reads and writes in place, and the numbers\n" +
		"// of the built-in atoms it names, from js_host_layout of engine/quickjs.c\n" +
		"// (JS_HOST_LAYOUT_* in engine/quickjs.h).\nconst (\n")
	w := 0
	for _, n := range HostLayoutNames {
		w = max(w, len(n))
	}
	for i, v := range vs {
		fmt.Fprintf(&b, "\t%-*s = %d\n", w, HostLayoutNames[i], v)
	}
	b.WriteString(")\n\n")
	return b.String()
}
