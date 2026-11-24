// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faulttest

// pattern is what fillAndFault writes below the stack pointer, each word
// exclusive-ored with its address.
const pattern = 0x5a5a5a5a5a5a5a5a

// fillAndFault writes its stack pointer to *top, fills [top-span, top-16)
// with pattern, and loads from wild, which must fault. It is assembly, so
// that nothing runs on the stack between the fill and the fault, for
// TestFaultStateFits.
func fillAndFault(wild uintptr, top *uintptr, span uintptr)

// written returns how far below top the lowest word of [top-span, top-16)
// that no longer holds the pattern of fillAndFault is, or 0 when every word
// does. It is assembly with no frame and no stack check, so the stack cannot
// move while it reads.
func written(top, span uintptr) uintptr
