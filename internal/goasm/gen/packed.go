// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"strings"
)

// PackedEntry is the suffix of the Go name of a packed function's generated
// entry, whose plain name goes to the Go function PackedGo writes.
const PackedEntry = "_block"

// PackedGo writes the Go function a host calls a packed function by. The
// pass gives a function whose arguments need more integer registers than a
// unified entry has (six on arm64, ten on amd64) a block of them instead:
// it then takes the shadow pointer and the block's address. The Go host
// calls the engine's API by the C signature on every target, so the function
// takes the arguments of was (the signature code the function had, the
// shadow pointer first), stores them in a block at the shadow pointer, each
// in an 8-byte slot in order (16 bytes for a pair) as the pass lays the block
// out, and calls entry with the shadow pointer above the block. The block
// fits: the engine keeps 64 KiB of the region free past any shadow pointer it
// gives out, and a block is a few words. The function panics where the
// region would end inside the block all the same, which no host does, since
// the engine checks the room its own frames take but not this.
//
// ok is false when was has an argument the host cannot pass by value (an
// aggregate passed by copy, a type the generator does not know): such a
// function gets no Go function, and a host call of it does not compile.
func PackedGo(name, entry, was string) (src string, ok bool, err error) {
	params, ret, found := strings.Cut(was, ">")
	if !found || len(params) == 0 || params[0] != 'i' {
		return "", false, fmt.Errorf("%s: signature %q does not start with the shadow pointer", name, was)
	}
	var args, stores []string
	off, w := 0, 1
	for _, c := range params[1:] {
		switch c {
		case 'i', 'n':
			args = append(args, fmt.Sprintf("a%d uintptr", w))
			stores = append(stores, fmt.Sprintf("\t*(*uintptr)(packedSlot(a0 + %d)) = a%d\n", off, w))
			off += 8
			w++
		case 'd':
			args = append(args, fmt.Sprintf("a%d float64", w))
			stores = append(stores, fmt.Sprintf("\t*(*float64)(packedSlot(a0 + %d)) = a%d\n", off, w))
			off += 8
			w++
		case 'j':
			args = append(args, fmt.Sprintf("a%d, a%d uintptr", w, w+1))
			stores = append(stores, fmt.Sprintf("\t*(*uintptr)(packedSlot(a0 + %d)) = a%d\n", off, w),
				fmt.Sprintf("\t*(*uintptr)(packedSlot(a0 + %d)) = a%d\n", off+8, w+1))
			off += 16
			w += 2
		default:
			return "", false, nil
		}
	}
	var res, call string
	switch ret {
	case "v":
		call = "\t%s(a0+%d, a0)\n"
	case "i", "n":
		res, call = " uintptr", "\treturn %s(a0+%d, a0)\n"
	case "d":
		res, call = " float64", "\treturn %s(a0+%d, a0)\n"
	case "j":
		res, call = " (r0, r1 uintptr)", "\treturn %s(a0+%d, a0)\n"
	default:
		return "", false, nil
	}
	size := align(off, 16)
	var b strings.Builder
	fmt.Fprintf(&b, "// %s stores its arguments in a block at the shadow pointer for\n// %s (gen.PackedGo).\n", name, entry)
	fmt.Fprintf(&b, "func %s(%s)%s {\n", name, strings.Join(append([]string{"a0 uintptr"}, args...), ", "), res)
	fmt.Fprintf(&b, "\tif (a0+%d)&(1<<shadowSizeLog2-1) < %d {\n\t\tpanic(\"cengine: the block of %s ends past the shadow-stack region\")\n\t}\n", size, size, name)
	for _, s := range stores {
		b.WriteString(s)
	}
	fmt.Fprintf(&b, call, entry, size)
	b.WriteString("}\n")
	return b.String(), true, nil
}

// PackedSlot is the helper PackedGo's functions store through: a uintptr
// that points outside the Go heap made a pointer without a conversion vet
// would question.
const PackedSlot = `// packedSlot makes a pointer of an address in the shadow-stack region.
func packedSlot(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }
`
