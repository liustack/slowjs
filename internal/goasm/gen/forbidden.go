// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// The instructions engine code may not hold: the ones that call the system
// or read what the machine says (the time stamp counter, the hardware random
// source, the processor's identity), and the traps. The plan's list is
// syscall, sysenter, int 0x80, svc, rdtsc, rdtscp, rdrand, rdseed and cpuid.
// A trap (ud2, int3, hlt, brk, udf) is SIGILL, SIGTRAP or a general
// protection fault, and in Go SIGILL and SIGTRAP end the process whatever
// the program does, so engine code reaches host_trap instead (see
// pass/shadow.cpp). The lowering reads the assembly text for them and the
// post-link check the decoded machine code (verify_insn.go), so a mistake in
// one of the two readings does not let one through. arm64 has no post-link
// check yet: EmitARM64 reads the words of the assembled object.
package gen

import (
	"regexp"

	"golang.org/x/arch/x86/x86asm"
)

// reForbidden matches an amd64 mnemonic in llc's AT&T text. The forms with a
// register operand carry a size suffix (rdrandq, rdseedl, sysretq), and any
// `int` takes an immediate.
var reForbidden = regexp.MustCompile(`\b(syscall|sysenter|sysexit[lq]?|sysret[lq]?|rdtscp?|rdrand[wlq]?|rdseed[wlq]?|` +
	`rdpmc|rdmsr|rd[fg]sbase[lq]?|cpuid|ud[012]|hlt)\b|\bint\s+\$|\bint3\b`)

// forbiddenA64 are the arm64 instructions that call the system or read or
// write machine state: the exception calls and mrs/msr, which also read the
// counter and the random number register. brk, udf and hlt are the traps.
var forbiddenA64 = map[string]bool{"svc": true, "hvc": true, "smc": true, "mrs": true, "msr": true,
	"brk": true, "udf": true, "hlt": true, "dcps1": true, "dcps2": true, "dcps3": true}

// a64ForbiddenWord says why an assembled arm64 instruction may not be in
// engine code, or "" when it may. The class of exception generation has
// 0xd4 in its top byte: svc, hvc, smc, brk, hlt and dcps1 to dcps3. udf is
// the reserved space whose top 16 bits are 0.
func a64ForbiddenWord(w uint32) string {
	switch {
	case w>>24 == 0xd4:
		return "an exception-generating instruction (svc, hvc, smc, brk, hlt or dcps)"
	case w>>16 == 0:
		return "udf, a permanently undefined instruction"
	}
	return ""
}

// systemOps are the decoded amd64 instructions that reach the system or read
// machine state. The post-link check refuses them wherever it decodes code,
// llc's and the linked binary's.
var systemOps = map[x86asm.Op]string{
	x86asm.SYSCALL: "a system call", x86asm.SYSENTER: "a system call", x86asm.SYSEXIT: "a system call",
	x86asm.SYSRET: "a system call", x86asm.INT: "a software interrupt",
	x86asm.RDTSC: "reads the time stamp counter", x86asm.RDTSCP: "reads the time stamp counter",
	x86asm.RDRAND: "reads the hardware random source", x86asm.RDPMC: "reads a performance counter",
	x86asm.RDMSR: "reads a machine register", x86asm.RDFSBASE: "reads a segment base", x86asm.RDGSBASE: "reads a segment base",
	x86asm.CPUID: "reads the processor's identity",
	// The traps. int3 decodes as INT above.
	x86asm.UD0: "a trap (undefined instruction)", x86asm.UD1: "a trap (undefined instruction)",
	x86asm.UD2: "a trap (undefined instruction)", x86asm.HLT: "a trap (halt)",
}

// isRDSEED reports the bytes of RDSEED: 0F C7 /7 with a register operand
// (mod 11), after the prefixes. x86asm has no RDSEED and cannot decode it, so
// decode names it before it reports an unknown instruction.
func isRDSEED(b []byte) bool {
	i := 0
	for i < len(b) && (b[i] == 0x66 || b[i] == 0xf2 || b[i] == 0xf3 || b[i]>>4 == 4) {
		i++
	}
	return i+2 < len(b) && b[i] == 0x0f && b[i+1] == 0xc7 && b[i+2]>>6 == 3 && b[i+2]>>3&7 == 7
}
