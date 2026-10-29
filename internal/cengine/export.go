// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

// What package slowjs needs of the engine besides the JS_ functions of
// engine_decl_<arch>.go, which it calls directly: the layout of values it reads and
// writes in place, the built-in atoms it names, the few lower-case functions
// it calls, and where a VM's runtime, context and stack are.

// The layout of values, and the built-in atoms, of the engine the generator
// wrote (engine_decl_<arch>.go).
const (
	RefCountOffset = layoutRefCountOffset // the int32 reference count, from the address a value holds
	StringLenMask  = layoutStringLenMask  // the length in the first uint32 of a string
	StringWideBit  = layoutStringWideBit  // set in it when the characters are 16-bit units
	StringChars    = layoutStringChars    // where the characters of a string start
	StringLenMax   = layoutStringLenMax   // the most characters a string holds

	AtomEmptyString  = atomEmptyString
	AtomFileName     = atomFileName
	AtomLineNumber   = atomLineNumber
	AtomColumnNumber = atomColumnNumber
	AtomMessage      = atomMessage
	AtomStack        = atomStack
	AtomName         = atomName
	AtomToString     = atomToString
	AtomThen         = atomThen
)

// ShadowSize is the size of the shadow stack of a VM, and ScratchSize the
// bytes at its base the host keeps: every entry starts above them.
const (
	ShadowSize  = 1 << shadowSizeLog2
	ScratchSize = scratchSize
)

// Malloc is js_malloc_rt: n bytes of the VM's heap, or 0.
func Malloc(sp, rt, n uintptr) uintptr { return js_malloc_rt(sp, rt, n) }

// FreeRT is js_free_rt, which frees what Malloc gave.
func FreeRT(sp, rt, p uintptr) { js_free_rt(sp, rt, p) }

// Free is js_free, which frees what the engine gave a caller of the context,
// such as the bytes of JS_WriteObject.
func Free(sp, ctx, p uintptr) { js_free(sp, ctx, p) }

// FreeValue is __JS_FreeValue, which frees a value whose reference count
// went to 0.
func FreeValue(sp, ctx, u, tag uintptr) { __JS_FreeValue(sp, ctx, u, tag) }

// ArrayOf is js_array_of, Array.of of the argc values at argv.
func ArrayOf(sp, ctx, argc, argv uintptr) (u, tag uintptr) {
	return js_array_of(sp, ctx, 0, tagUndefined, argc, argv)
}

// Runtime and Context are the JSRuntime and the JSContext of the VM.
func (v *VM) Runtime() uintptr { return v.rt }
func (v *VM) Context() uintptr { return v.ctx }

// SP is where each entry into the engine starts on the shadow stack.
func (v *VM) SP() uintptr { return v.sp }
