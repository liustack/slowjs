// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// The rewrites the lowering makes, written down per function as data. The
// post-link check (verify_insn.go) accepts a difference between llc's object
// and the linked code only where one of these declares it, and checks the
// declared rewrite against llc's object itself.
package gen

// FuncRecord is every rewrite the lowering made in one function.
type FuncRecord struct {
	Name                  string
	Kind                  Kind
	Sig                   Sig
	PreserveNone, SaveAll bool
	// The canonical prologue the lowering removed: the callee-saved
	// registers it pushed, in order, and the bytes it reserved below them.
	Pushed []string
	Local  int
	// The Go frame: locals at LocalBase, saved registers at SaveBase, the
	// outgoing argument area at 0, Frame bytes in all.
	LocalBase, SaveBase, Frame int
	// Saved are the registers saved around every unified call.
	Saved []string
	// Calls and Refs are in llc's order: every call, and every RIP-relative
	// reference to data or to a function.
	Calls []CallRecord
	Refs  []RefRecord
}

// CallKind says how the lowering rewrote one call.
type CallKind int

const (
	CallUnified CallKind = iota // a direct call to a unified entry, marshalled
	CallLeaf                    // a direct call to a register-entry leaf, kept
	CallReg                     // a call through a register, marshalled
)

// CallRecord is one call: its target (a C symbol, or the register of CallReg)
// and the signature it was marshalled with.
type CallRecord struct {
	Kind   CallKind
	Target string
	Sig    Sig
}

// RefRecord is one RIP-relative reference as llc's text wrote it, label plus
// addend, and where the lowering pointed it: a function, engine_rodata, or
// the function's own copy of the data.
type RefRecord struct {
	Label      string
	Add        int64
	Func, Blob bool
}
