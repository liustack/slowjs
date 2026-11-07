// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"encoding/binary"
	"strings"
	"testing"
)

// amd64Prologue is the start of a split function on linux/amd64 for the TEXT
// frame frame: the load of g, the stack check, BP and the frame. The check is
// the compare of SP with the guard, or with R12 after LEAQ disp(SP), R12.
func amd64Prologue(frame int, lea bool, disp int32) []byte {
	b := []byte{0x64, 0x4c, 0x8b, 0x34, 0x25, 0xf8, 0xff, 0xff, 0xff} // MOVQ FS:-8, R14
	if lea {
		b = append(b, 0x4c, 0x8d, 0xa4, 0x24) // LEAQ disp(SP), R12
		b = binary.LittleEndian.AppendUint32(b, uint32(disp))
		b = append(b, 0x4d, 0x3b, 0x66, 0x10) // CMPQ R12, 16(R14)
	} else {
		b = append(b, 0x49, 0x3b, 0x66, 0x10) // CMPQ SP, 16(R14)
	}
	b = append(b, 0x0f, 0x86, 0x00, 0x01, 0x00, 0x00) // JBE morestack
	b = append(b, 0x55, 0x48, 0x89, 0xe5)             // PUSHQ BP; MOVQ SP, BP
	switch {
	case frame == 0:
	case frame < 128:
		b = append(b, 0x48, 0x83, 0xec, byte(frame)) // SUBQ $frame, SP
	default:
		b = append(b, 0x48, 0x81, 0xec) // SUBQ $frame, SP
		b = binary.LittleEndian.AppendUint32(b, uint32(frame))
	}
	return append(b, make([]byte, 32)...)
}

// TestGoPrologueStackCheck: the stack check of a prologue is the one Go's
// assembler writes for its frame (the TEXT size and the 8 bytes of BP): SP
// against the guard up to 128 bytes, SP less (frame - 128) up to 4096. A
// displacement that checks less depth, and a LEA a small frame does not have,
// are refused.
func TestGoPrologueStackCheck(t *testing.T) {
	const at = 0x1000
	for _, c := range []struct {
		name  string
		frame int // the TEXT frame, 8 less than the frame Go checks for
		lea   bool
		disp  int32
		want  string
	}{
		{"no frame", 0, false, 0, ""},
		{"a small frame", 112, false, 0, ""},
		{"the largest small frame", 120, false, 0, ""},
		{"the smallest big frame", 128, true, -8, ""},
		{"a frame of 496, the interpreter's", 496, true, -376, ""},
		{"the largest big frame", 4088, true, -3968, ""},
		{"a frame past 4096", 4096, true, -3976, "past the 4096"},
		{"a displacement of 0", 496, true, 0, "takes SP less 0"},
		{"a displacement 8 too small", 496, true, -368, "takes SP less 368, want less 376"},
		{"a displacement 8 too big", 496, true, -384, "takes SP less 384, want less 376"},
		{"the displacement of the whole frame", 496, true, -504, "takes SP less 504"},
		{"a positive displacement", 496, true, 376, "takes SP less -376"},
		{"a big frame with no LEA", 496, false, 0, "takes SP less 0"},
		{"a LEA for a small frame", 112, true, -8, "which is checked against SP itself"},
	} {
		code := amd64Prologue(c.frame, c.lea, c.disp)
		b := &binFile{format: "test", goos: "linux", secs: []*binSec{{name: "text", addr: at, size: uint64(len(code)), data: code}}}
		k := &checker{bin: b}
		g, err := k.goPrologue(at, false)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want == "" && (!g.split || !g.bp || g.frame != int64(c.frame)):
			t.Errorf("%s: read split %v, BP %v, frame %d", c.name, g.split, g.bp, g.frame)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

// amd64HostPoll is host_poll at 0x1000 as the Go compiler makes it for
// linux/amd64 (the checked-in engine): the stack check, a frame with a call,
// then after the RET the spill of the argument, a 2-byte no-op, the call of
// morestack, the reload and the JMP to the start. It is 0x2c bytes.
func amd64HostPoll() (*checker, []byte) {
	const at, morestack, poll = 0x1000, 0x2000, 0x3000
	rel := func(from, to int) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(to-from)) }
	code := []byte{
		0x49, 0x3b, 0x66, 0x10, // 0x00 CMPQ SP, 16(R14)
		0x76, 0x13, // 0x04 JBE 0x19
		0x55, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x08, // PUSHQ BP; MOVQ SP, BP; SUBQ $8, SP
		0xe8, // 0x0e CALL gohost.Poll
	}
	code = append(code, rel(at+0x13, poll)...)
	code = append(code, 0x48, 0x83, 0xc4, 0x08, 0x5d, 0xc3) // 0x13 ADDQ $8, SP; POPQ BP; RET
	code = append(code, 0x48, 0x89, 0x44, 0x24, 0x08)       // 0x19 MOVQ AX, 8(SP)
	code = append(code, 0x66, 0x90)                         // 0x1e NOPW
	code = append(code, 0xe8)                               // 0x20 CALL morestack
	code = append(code, rel(at+0x25, morestack)...)
	code = append(code, 0x48, 0x8b, 0x44, 0x24, 0x08) // 0x25 MOVQ 8(SP), AX
	code = append(code, 0xeb, 0xd4)                   // 0x2a JMP 0x00
	size := len(code)
	code = append(code, make([]byte, 32)...)
	b := &binFile{format: "test", goos: "linux",
		syms: map[string]binSym{"p.host_poll": {Value: at, Size: uint64(size)}, "runtime.morestack_noctxt.abi0": {Value: morestack}},
		secs: []*binSec{{name: "text", addr: at, size: uint64(len(code)), data: code}}}
	return &checker{bin: b, c: EngineCheck{Pkg: "p"}}, code
}

// TestCheckHostPollStraightBlock: the block that host_poll's stack check
// branches to must call morestack and jump back to the start in a straight
// run. A jump that skips the call, a call of something else, a RET and a call
// that is not there are refused as the arm64 check refuses them.
func TestCheckHostPollStraightBlock(t *testing.T) {
	k, _ := amd64HostPoll()
	if err := k.checkHostPoll(); err != nil {
		t.Fatalf("host_poll as the compiler makes it: %v", err)
	}
	for _, c := range []struct {
		name string
		edit func(code []byte)
		want string
	}{
		{"a JZ over the call", func(c []byte) { copy(c[0x1e:], []byte{0x74, 0x05}) }, "branch of its own"},
		{"a JNE over the call", func(c []byte) { copy(c[0x1e:], []byte{0x75, 0x05}) }, "branch of its own"},
		{"a JRCXZ over the call", func(c []byte) { copy(c[0x1e:], []byte{0xe3, 0x05}) }, "branch of its own"},
		{"a JMP over the call", func(c []byte) { copy(c[0x1e:], []byte{0xeb, 0x05}) }, "does not call morestack"},
		{"an indirect JMP in the block", func(c []byte) { copy(c[0x1e:], []byte{0xff, 0xe0}) }, "branch of its own"},
		{"a RET in the block", func(c []byte) { c[0x1e] = 0xc3 }, "branch of its own"},
		{"a conditional jump after the call", func(c []byte) { copy(c[0x2a:], []byte{0x74, 0xd4}) }, "branch of its own"},
		{"the call of another function", func(c []byte) { copy(c[0x21:], binary.LittleEndian.AppendUint32(nil, 0x1000)) }, "something else than morestack"},
		{"no call", func(c []byte) { copy(c[0x20:], []byte{0x0f, 0x1f, 0x44, 0x00, 0x00}) }, "does not call morestack"},
		{"the jump back elsewhere", func(c []byte) { c[0x2b] = 0xd5 }, "does not call morestack"},
		{"no jump back", func(c []byte) { copy(c[0x2a:], []byte{0x66, 0x90}) }, "does not call morestack"},
		{"no stack check", func(c []byte) { copy(c[0x00:], []byte{0x90, 0x90, 0x90, 0x90}) }, "host_poll has no stack check"},
		{"the branch of the check outside", func(c []byte) { c[0x05] = 0x70 }, "outside the function"},
	} {
		k, code := amd64HostPoll()
		c.edit(code)
		k.bin.secs[0].data = code
		if err := k.checkHostPoll(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}
