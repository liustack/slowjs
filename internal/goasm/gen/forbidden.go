// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// The instructions engine code may not hold: the ones that call the system
// or read what the machine says (the time stamp counter, the hardware random
// source, the processor's identity). The plan's list is syscall, sysenter,
// int 0x80, svc, rdtsc, rdtscp, rdrand, rdseed and cpuid. The lowering reads
// the assembly text for them and the post-link check the decoded machine code
// (verify_insn.go), so a mistake in one of the two readings does not let one
// through.
package gen

import (
	"regexp"

	"golang.org/x/arch/x86/x86asm"
)

// reForbidden matches an amd64 mnemonic in llc's AT&T text. The forms with a
// register operand carry a size suffix (rdrandq, rdseedl, sysretq), and any
// `int` takes an immediate.
var reForbidden = regexp.MustCompile(`\b(syscall|sysenter|sysexit[lq]?|sysret[lq]?|rdtscp?|rdrand[wlq]?|rdseed[wlq]?|` +
	`rdpmc|rdmsr|rd[fg]sbase[lq]?|cpuid)\b|\bint\s+\$|\bint3\b`)

// forbiddenA64 are the arm64 instructions that call the system or read or
// write machine state: the exception calls and mrs/msr, which also read the
// counter and the random number register.
var forbiddenA64 = map[string]bool{"svc": true, "hvc": true, "smc": true, "mrs": true, "msr": true}

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
}
