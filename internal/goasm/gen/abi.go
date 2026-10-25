// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Package gen is the phase-0 prototype of the new C-source engine generator.
// It is architecture independent in structure: an Arch describes the Go frame
// and the native calling convention, and the lowering and checks are written
// against that interface so arm64 can be added without touching the pipeline.
//
// This file holds the shared vocabulary: the two function entry kinds, the
// value types the engine passes between functions, and the amd64 Arch.
package gen

// Kind is one of the two entry conventions of the plan.
type Kind int

const (
	// Unified entry: arguments arrive on the Go stack (ABI0), the body reads
	// them after Go's split-check prologue, and the caller relies on no
	// callee-saved register surviving the call. The shadow-stack pointer is
	// the first stack argument. Function tables, address comparisons and
	// indirect calls all use this entry.
	Unified Kind = iota
	// Register entry: a NOSPLIT leaf that neither grows the stack nor calls a
	// split-checked function. Arguments stay in the native argument registers.
	// Only a direct call to a known leaf may target it.
	Register
)

// Type is a value a generated function passes or returns. Every type maps to
// non-Go memory, so the paired Go declaration uses uintptr/int64/float64 and
// carries no Go pointer: the argument area needs no pointer map beyond the
// empty one.
type Type int

const (
	Void Type = iota
	I64       // 64-bit integer, one GP register / one stack word
	Ptr       // engine pointer, treated as uintptr, one GP register / word
	F64       // double, one SSE register / one stack word
	JSV       // JSValue, 16 bytes, two GP registers, two stack words
)

func (t Type) words() int {
	switch t {
	case Void:
		return 0
	case JSV:
		return 2
	default:
		return 1
	}
}

func (t Type) bytes() int { return t.words() * 8 }

// Sig is the signature of a generated function in logical order. By convention
// Params[0] of a Unified function is the shadow-stack pointer (Ptr).
type Sig struct {
	Params []Type
	Ret    Type
}

// Arch abstracts the target so the lowering and checks are architecture
// independent. amd64 is implemented here; arm64 is a separate Arch.
type Arch interface {
	Name() string
	// IntArgRegs / SSEArgRegs give the native argument registers in order.
	IntArgRegs() []string
	SSEArgRegs() []string
	// CalleeSaved are the native callee-saved GP registers a body may hold a
	// value in. A unified call clobbers them, so the caller saves the live
	// ones to its frame.
	CalleeSaved() []string
	// RetIntRegs / RetSSEReg give where a native body leaves its result.
	RetIntRegs() []string
	RetSSEReg() string
	// GoFramePrologueBytes is the distance Go's prologue adds between the
	// frame base and the incoming argument area (return address + saved BP).
	GoFramePrologueBytes() int
}

// amd64 implements Arch for linux/darwin/windows amd64 (GOAMD64=v1).
type AMD64Arch struct{}

func AMD64() AMD64Arch { return AMD64Arch{} }

func (AMD64Arch) Name() string          { return "amd64" }
func (AMD64Arch) IntArgRegs() []string  { return []string{"di", "si", "dx", "cx", "r8", "r9"} }
func (AMD64Arch) SSEArgRegs() []string  { return []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5", "xmm6", "xmm7"} }
func (AMD64Arch) CalleeSaved() []string { return []string{"rbx", "r12", "r13", "r14", "r15"} }
func (AMD64Arch) RetIntRegs() []string  { return []string{"rax", "rdx"} }
func (AMD64Arch) RetSSEReg() string     { return "xmm0" }

// Return address (8) plus the base pointer Go pushes (8): the incoming
// argument area sits that far above SP once the prologue has run.
func (AMD64Arch) GoFramePrologueBytes() int { return 16 }
