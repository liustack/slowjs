// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The jump tables of arm64 after the lowering.
//
// llc writes a jump table as a run of entries in the data of the function,
// each the distance from a base label to a target label of its code:
//
//	.word  LBB - Lbase          4 bytes, read by ldrsw, any distance
//	.hword (LBB - Lbase)>>2     2 bytes, read by ldrh, at most 65535 steps of 4
//	.byte  (LBB - Lbase)>>2     1 byte, read by ldrb, at most 255 steps of 4
//
// It picks the narrow forms (AArch64CompressJumpTables) from the sizes of its
// own code. The lowering then adds code inside the function (the calls, the
// polls, the frame), so a narrow table llc sized can end too narrow for the
// lowered code. llvm-mc refuses an entry past its width today, and the
// generator turns the narrow forms off (-aarch64-enable-compress-jump-tables=false,
// cmd/engine). This check does not rest on either: it sizes the lowered code
// itself and refuses any entry whose distance does not fit its width, or that
// is not a whole number of steps where it is shifted, naming the function.
//
// The size of the code: every instruction of the lowered text is 4 bytes (no
// line is a macro llvm-mc expands), a label is 0, and .p2align N pads to a
// multiple of 2^N, unless the padding would pass its max-skip. FuncPlace puts
// a function at a multiple of 16 bytes, or at its Align64 place (a multiple
// of 64 and a skip), so the offsets count from that place, and an alignment
// above the one FuncPlace keeps is refused. Anything else in the code is
// refused too, so that the sizes never go wrong without a word.

var (
	reA64JTEntry = regexp.MustCompile(`^\.(byte|hword|short|word|long|int)\s+(.+)$`)
	reA64JTDiff  = regexp.MustCompile(`^\(?\s*([.\w$]+)\s*-\s*([.\w$]+)\s*\)?(\s*>>\s*2)?$`)
	reA64P2Align = regexp.MustCompile(`^\.p2align\s+(\d+)(?:\s*,\s*([^,]*?))?(?:\s*,\s*(\d+))?$`)
)

// a64FuncStart is where FuncPlace puts function name: start bytes after a
// multiple of 2^align.
func a64FuncStart(align64 map[string]int, name string) (start, align int) {
	if off, ok := align64[name]; ok {
		return off, 6
	}
	return 0, 4
}

// a64CodeLabels gives the offset of each label of the lowered code, counted
// from a multiple of 2^align, where the function starts start bytes after it.
func a64CodeLabels(code []string, start, align int) (map[string]int, error) {
	at := map[string]int{}
	off := start
	for _, entry := range code {
		for _, l := range strings.Split(entry, "\n") {
			l = strings.TrimSpace(l)
			switch {
			case l == "":
			case isA64Label(l):
				at[strings.TrimSuffix(l, ":")] = off
			case strings.HasPrefix(l, ".p2align"):
				m := reA64P2Align.FindStringSubmatch(l)
				if m == nil {
					return nil, fmt.Errorf("cannot size %q", l)
				}
				n, _ := strconv.Atoi(m[1])
				if n > align {
					return nil, fmt.Errorf("an alignment of 2^%d in the code, more than the 2^%d the function is placed on", n, align)
				}
				step := 1 << n
				pad := (off+step-1)&^(step-1) - off
				if m[3] != "" {
					if max, _ := strconv.Atoi(m[3]); pad > max {
						pad = 0
					}
				}
				off += pad
			case strings.HasPrefix(l, ".") || strings.HasPrefix(l, "@"):
				return nil, fmt.Errorf("cannot size %q in the code", l)
			default:
				off += 4
			}
		}
	}
	return at, nil
}

// a64CheckJumpTables refuses an entry of a jump table in data (the data of
// the function's body) whose distance between two labels of the lowered code
// does not fit its width. The function starts start bytes after a multiple of
// 2^align (a64FuncStart). An entry of data that names no code label is not a
// jump table and is left alone.
func a64CheckJumpTables(code []string, data string, start, align int) error {
	if data == "" {
		return nil
	}
	var at map[string]int
	table := ""
	for _, l := range strings.Split(data, "\n") {
		l = strings.TrimSpace(l)
		if isA64Label(l) {
			table = strings.TrimSuffix(l, ":")
			continue
		}
		m := reA64JTEntry.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		d := reA64JTDiff.FindStringSubmatch(strings.TrimSpace(m[2]))
		if d == nil {
			continue
		}
		if at == nil {
			var err error
			if at, err = a64CodeLabels(code, start, align); err != nil {
				return fmt.Errorf("jump table %s: %v", table, err)
			}
		}
		to, ok1 := at[d[1]]
		from, ok2 := at[d[2]]
		if !ok1 && !ok2 {
			continue
		}
		dist, shifted := to-from, d[3] != ""
		var width int
		switch m[1] {
		case "byte":
			width = 8
		case "hword", "short":
			width = 16
		default:
			width = 32
		}
		if !ok1 || !ok2 {
			// A table of whole distances from a label of the data, as the
			// computed goto of JS_RegvmRun has: 32 bits hold any distance
			// inside one function, a narrower entry is not sized here.
			if width == 32 && !shifted {
				continue
			}
			return fmt.Errorf("jump table %s: %q names a label that is not in the code", table, l)
		}
		v := dist
		if shifted {
			if dist < 0 || dist%4 != 0 {
				return fmt.Errorf("jump table %s: %q is %d bytes after the lowering, not a whole number of 4-byte steps forward", table, l, dist)
			}
			v = dist / 4
			if v >= 1<<width {
				return fmt.Errorf("jump table %s: %q is %d steps of 4 bytes after the lowering, past the %d a .%s holds: the lowering grew the code after llc sized the table (see -aarch64-enable-compress-jump-tables in cmd/engine)", table, l, v, 1<<width-1, m[1])
			}
			continue
		}
		if v < -(1<<(width-1)) || v >= 1<<(width-1) {
			return fmt.Errorf("jump table %s: %q is %d bytes after the lowering, past what a .%s holds", table, l, v, m[1])
		}
	}
	return nil
}
