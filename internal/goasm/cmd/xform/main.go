// Copyright (c) 2026 The liustack/quickjs Authors.
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
//	shift:N[:NAME]         N bytes of NOP after the last RET of function
//	                       NAME (without NAME, after the first RET of
//	                       cutils_amd64.s), where they never run, so every
//	                       function laid out after it moves by N bytes. NAME
//	                       must be a function the linker keeps: the first
//	                       function of cutils_amd64.s is never called, so the
//	                       linker drops it with its NOPs and a shift without
//	                       NAME moves no code of the C engine. measure bench
//	                       build passes the engine function that the formal
//	                       binary places first. Go puts amd64 functions on 32
//	                       bytes, so use a multiple of 32. A function the
//	                       generator puts on 64 bytes moves by a multiple of
//	                       64.
//	front:NAME:N           N bytes of multi-byte NOP at the start of the body
//	                       of function NAME, right after its PCALIGN, which
//	                       run at each call. They move the code of that
//	                       function only, with the data after its code. Use a
//	                       multiple of 16 from 0 to 48 on the interpreter
//	                       (JS_RegvmRun): its constant pools stay on 16
//	                       bytes, and a front that is not a multiple of 16 is
//	                       refused on a function with data after its code.
//	endalign:A[:F1,F2,..]  PCALIGN $A after the RET that ends a function (all
//	                       functions, or only the named): the linker then
//	                       starts the function on a multiple of A.
//	body:A:F1,F2,..        PCALIGN $A in place of the PCALIGN at the start of
//	                       the body of the named functions.
//
// Example, the position "moved 32 bytes, 16 NOP bytes in front of the
// interpreter":
//
//	go run ./internal/goasm/cmd/xform copy-of-generated-dir shift:32 front:JS_RegvmRun:16
package main

import (
	"bytes"
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

// head matches the start of a function NAME: its TEXT line, the lines the
// generator writes before the body (a NO_LOCAL_POINTERS line, and the block
// "#ifdef GOOS_windows ... #endif" with the room check of windows/amd64, whose
// lines are indented) and the PCALIGN of the body. Group 1 is everything up to
// the PCALIGN, group 2 the white space before it. Lines of the block must be
// indented, so a match never runs on into the next function.
func head(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^(TEXT ·` + regexp.QuoteMeta(name) + `\(SB\).*\n(?:[ \t]*NO_LOCAL_POINTERS\n)?(?:#ifdef [^\n]*\n(?:[ \t][^\n]*\n)*#endif\n)?)([ \t]*)PCALIGN \$\d+\n`)
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
			if len(parts) == 3 {
				if err := padAfter(files, parts[2], n); err != nil {
					return fmt.Errorf("%s: %w", op, err)
				}
				continue
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
				// The data after the code of the function (its constant pools
				// and jump tables) moves with the body, and the generator keeps
				// a pool on 16 bytes for the SSE instructions that read it with
				// a PCALIGN $16 that adds nothing where the body sits on its
				// 16 bytes. A front off 16 makes that PCALIGN pad, and the data
				// moves away from the displacements the code reads it by.
				fn := s[m[1]:]
				if k := bytes.Index(fn, []byte("\nTEXT ")); k >= 0 {
					fn = fn[:k]
				}
				if n%16 != 0 && bytes.Contains(fn, []byte("\n\t// data\n")) {
					return fmt.Errorf("%s: %s keeps data after its code, which the front moves with the body, and %d bytes would make the PCALIGN $16 in front of the data pad it away from the code that reads it: use a multiple of 16", op, parts[1], n)
				}
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

// padAfter puts n bytes of NOP after the last RET of function name, in the one
// file of files that has it.
func padAfter(files []string, name string, n int) error {
	hit := 0
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		s := string(b)
		starts := textLine.FindAllStringSubmatchIndex(s, -1)
		for i, st := range starts {
			if s[st[2]:st[3]] != name {
				continue
			}
			end := len(s)
			if i+1 < len(starts) {
				end = starts[i+1][0]
			}
			k := strings.LastIndex(s[st[0]:end], "\tRET\n")
			if k < 0 {
				return fmt.Errorf("no RET in %s", name)
			}
			k += st[0] + len("\tRET\n")
			s = s[:k] + strings.Repeat("\tBYTE $0x90\n", n) + s[k:]
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				return err
			}
			hit++
			break
		}
	}
	if hit != 1 {
		return fmt.Errorf("function %s found %d times", name, hit)
	}
	return nil
}
