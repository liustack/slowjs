// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command xform moves the code of generated Go assembly, to measure how much a
// speed result depends on where the code sits (see "Code positions" in
// docs/performance.md).
//
//	go run ./internal/goasm/cmd/xform DIR OP...
//
// It edits the *_amd64.s files of DIR in place, so run it on a copy, never on
// the committed files. The OPs are applied in order:
//
//	shift:N                N bytes of NOP after the RET of the first function
//	                       of cutils_amd64.s, where they never run, so every
//	                       function laid out after it moves by N bytes. Go
//	                       puts amd64 functions on 32 bytes, so N below 32
//	                       moves nothing: use 0 or 32. A function the
//	                       generator puts on 64 bytes does not move.
//	front:NAME:N           N bytes of multi-byte NOP at the start of the body
//	                       of function NAME, right after its PCALIGN, which
//	                       run at each call. They move the code of that
//	                       function only. Use 0, 8, 16 or 24 on the
//	                       interpreter (JS_CallInternal in the C engine,
//	                       goasmRun in lib/goasm_run_linux_amd64.s).
//	endalign:A[:F1,F2,..]  PCALIGN $A after the RET that ends a function (all
//	                       functions, or only the named): the linker then
//	                       starts the function on a multiple of A.
//	body:A:F1,F2,..        PCALIGN $A in place of the PCALIGN at the start of
//	                       the body of the named functions.
//
// Example, the position "moved 32 bytes, 16 NOP bytes in front of the
// interpreter":
//
//	go run ./internal/goasm/cmd/xform copy-of-generated-dir shift:32 front:JS_CallInternal:16
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// nops are the multi-byte no-ops of 1 to 8 bytes, as the Intel and AMD
// manuals give them.
var nops = [][]byte{
	1: {0x90},
	2: {0x66, 0x90},
	3: {0x0f, 0x1f, 0x00},
	4: {0x0f, 0x1f, 0x40, 0x00},
	5: {0x0f, 0x1f, 0x44, 0x00, 0x00},
	6: {0x66, 0x0f, 0x1f, 0x44, 0x00, 0x00},
	7: {0x0f, 0x1f, 0x80, 0, 0, 0, 0},
	8: {0x0f, 0x1f, 0x84, 0, 0, 0, 0, 0},
}

// nopBytes writes n bytes of no-ops as BYTE lines, in no-ops of 8 bytes and
// one shorter one at the end.
func nopBytes(n int) string {
	var b strings.Builder
	for n > 0 {
		k := min(n, 8)
		for _, x := range nops[k] {
			fmt.Fprintf(&b, "\tBYTE $0x%02x\n", x)
		}
		n -= k
	}
	return b.String()
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: xform DIR OP... (see go doc ./internal/goasm/cmd/xform)")
		os.Exit(2)
	}
	if err := apply(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "xform:", err)
		os.Exit(1)
	}
}

// head matches the start of a function NAME: its TEXT line, the
// NO_LOCAL_POINTERS line it may have and the PCALIGN of its body, with or
// without the tab lib/goasm_run_linux_amd64.s leaves out.
func head(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^(TEXT ·` + regexp.QuoteMeta(name) + `\(SB\).*\n(?:[ \t]*NO_LOCAL_POINTERS\n)?)([ \t]*)PCALIGN \$\d+\n`)
}

var textLine = regexp.MustCompile(`(?m)^TEXT ·([A-Za-z0-9_$.]+)\(SB\)`)

func apply(dir string, ops []string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*_amd64.s"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, op := range ops {
		parts := strings.Split(op, ":")
		num := func(i int) (int, error) {
			if i >= len(parts) {
				return 0, fmt.Errorf("%s: too few fields", op)
			}
			n, err := strconv.Atoi(parts[i])
			if err != nil || n < 0 {
				return 0, fmt.Errorf("%s: %q is not a size", op, parts[i])
			}
			return n, nil
		}
		names := func(i int) map[string]bool {
			if i >= len(parts) || parts[i] == "" {
				return nil
			}
			m := map[string]bool{}
			for _, s := range strings.Split(parts[i], ",") {
				m[s] = true
			}
			return m
		}
		switch parts[0] {
		case "shift":
			n, err := num(1)
			if err != nil {
				return err
			}
			p := filepath.Join(dir, "cutils_amd64.s")
			s, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			i := strings.Index(string(s), "\tRET\n")
			if i < 0 {
				return fmt.Errorf("%s: no RET in %s", op, p)
			}
			i += len("\tRET\n")
			pad := strings.Repeat("\tBYTE $0x90\n", n)
			if err := os.WriteFile(p, []byte(string(s[:i])+pad+string(s[i:])), 0o644); err != nil {
				return err
			}
		case "front":
			if len(parts) != 3 {
				return fmt.Errorf("%s: want front:NAME:N", op)
			}
			n, err := num(2)
			if err != nil {
				return err
			}
			re := head(parts[1])
			hit := 0
			for _, p := range files {
				s, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				m := re.FindIndex(s)
				if m == nil {
					continue
				}
				hit++
				out := string(s[:m[1]]) + nopBytes(n) + string(s[m[1]:])
				if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
					return err
				}
			}
			if hit != 1 {
				return fmt.Errorf("%s: the body of %s found %d times", op, parts[1], hit)
			}
		case "endalign":
			a, err := num(1)
			if err != nil {
				return err
			}
			only := names(2)
			count := 0
			for _, p := range files {
				if filepath.Base(p) == "engine_rodata_amd64.s" {
					continue
				}
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				s := string(b)
				starts := textLine.FindAllStringSubmatchIndex(s, -1)
				if len(starts) == 0 {
					continue
				}
				var out strings.Builder
				out.WriteString(s[:starts[0][0]])
				for i, st := range starts {
					end := len(s)
					if i+1 < len(starts) {
						end = starts[i+1][0]
					}
					fn := s[st[0]:end]
					if only == nil || only[s[st[2]:st[3]]] {
						k := strings.LastIndex(fn, "\tRET\n")
						if k < 0 {
							return fmt.Errorf("%s: no RET in %s", op, s[st[2]:st[3]])
						}
						k += len("\tRET\n")
						fn = fn[:k] + fmt.Sprintf("\tPCALIGN $%d\n", a) + fn[k:]
						count++
					}
					out.WriteString(fn)
				}
				if err := os.WriteFile(p, []byte(out.String()), 0o644); err != nil {
					return err
				}
			}
			if only != nil && count != len(only) {
				return fmt.Errorf("%s: %d of the %d functions found", op, count, len(only))
			}
			fmt.Printf("endalign %d on %d functions\n", a, count)
		case "body":
			a, err := num(1)
			if err != nil {
				return err
			}
			only := names(2)
			if only == nil {
				return fmt.Errorf("%s: want body:A:NAME,...", op)
			}
			hit := 0
			for _, p := range files {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				s := string(b)
				changed := false
				for name := range only {
					re := head(name)
					m := re.FindStringSubmatchIndex(s)
					if m == nil {
						continue
					}
					s = s[:m[0]] + s[m[2]:m[3]] + s[m[4]:m[5]] + fmt.Sprintf("PCALIGN $%d\n", a) + s[m[1]:]
					hit++
					changed = true
				}
				if changed {
					if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
						return err
					}
				}
			}
			if hit != len(only) {
				return fmt.Errorf("%s: %d of the %d functions found", op, hit, len(only))
			}
		default:
			return fmt.Errorf("unknown op %s", op)
		}
	}
	return nil
}
