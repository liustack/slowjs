// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bufio"
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// HotOps are the opcodes whose handlers Addrs reports: the ones that take the
// most instructions in tifa's pages.
var HotOps = []string{"get_loc0", "call", "call_method", "return_undef", "get_field", "get_field2", "strict_eq", "put_field", "if_true", "if_false8"}

// Layout is where the code of the interpreter landed in one binary.
type Layout struct {
	Func     uint64            // the start of the function
	Body     uint64            // the first instruction after the NOPs at its start
	Size     uint64            // bytes of the function
	Handlers map[string]uint64 // the start of the handler of each hot opcode
}

// OpcodeNames reads the names of the opcodes, in the order of their numbers,
// from engine/quickjs-opcode.h.
func OpcodeNames(r io.Reader) ([]string, error) {
	re := regexp.MustCompile(`^DEF\(\s*([a-z0-9_]+)\s*,`)
	var names []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			names = append(names, m[1])
		}
	}
	return names, sc.Err()
}

// skipNops returns how many bytes of no-ops code starts with, in the encodings
// of xform and of the Go assembler's PCALIGN.
func skipNops(code []byte) int {
	i := 0
	for i < len(code) {
		n := 0
		for k := 8; k >= 1; k-- {
			if bytes.HasPrefix(code[i:], nops[k]) {
				n = k
				break
			}
		}
		if n == 0 {
			return i
		}
		i += n
	}
	return i
}

// nops are the multi-byte no-ops of 1 to 8 bytes, as xform writes them.
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

// ReadLayout finds the interpreter fn (such as JS_CallInternal_block) in the
// amd64 ELF binary, the first instruction of its body, and, with the opcode
// names, the handler of each of HotOps from its jump table: the most common
// target of a lea disp(%rip),%r11 in the function, 256 offsets of 4 bytes from
// its own start.
func ReadLayout(bin, fn string, names []string) (Layout, error) {
	f, err := elf.Open(bin)
	if err != nil {
		return Layout{}, err
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		return Layout{}, err
	}
	var s elf.Symbol
	for _, x := range syms {
		if strings.HasSuffix(x.Name, "."+fn+".abi0") || strings.HasSuffix(x.Name, "."+fn) {
			s = x
			break
		}
	}
	if s.Value == 0 {
		return Layout{}, fmt.Errorf("%s: no symbol %s", bin, fn)
	}
	text := f.Section(".text")
	if text == nil {
		return Layout{}, fmt.Errorf("%s: no .text", bin)
	}
	data, err := text.Data()
	if err != nil {
		return Layout{}, err
	}
	if s.Value < text.Addr || s.Value+s.Size > text.Addr+uint64(len(data)) {
		return Layout{}, fmt.Errorf("%s: %s is outside .text", bin, fn)
	}
	at := func(a uint64) []byte { return data[a-text.Addr:] }
	code := at(s.Value)[:s.Size]
	l := Layout{Func: s.Value, Size: s.Size, Body: s.Value + bodyStart(s.Value, code), Handlers: map[string]uint64{}}
	if len(names) == 0 {
		return l, nil
	}
	count := map[uint64]int{}
	for i := 0; i+7 <= len(code); i++ {
		if code[i] == 0x4c && code[i+1] == 0x8d && code[i+2] == 0x1d {
			disp := int32(binary.LittleEndian.Uint32(code[i+3:]))
			count[uint64(int64(s.Value)+int64(i)+7+int64(disp))]++
		}
	}
	var base uint64
	for t, c := range count {
		if c > count[base] || (c == count[base] && t < base) {
			base = t
		}
	}
	if base < text.Addr || base+256*4 > text.Addr+uint64(len(data)) {
		return l, fmt.Errorf("%s: the jump table of %s is not in .text", bin, fn)
	}
	tb := at(base)
	for _, op := range HotOps {
		for i, n := range names {
			if n == op && i < 256 {
				l.Handlers[op] = uint64(int64(base) + int64(int32(binary.LittleEndian.Uint32(tb[i*4:]))))
			}
		}
	}
	return l, nil
}

// LayoutHeader is the first line of the layouts file.
func LayoutHeader() string {
	cols := []string{"binary", "func", "body", "body%64", "body%4096"}
	for _, op := range HotOps {
		cols = append(cols, op+"%64", op+"%4096")
	}
	return strings.Join(cols, "\t")
}

// Row is the line of the layouts file of one binary.
func (l Layout) Row(name string) string {
	cols := []string{name, fmt.Sprintf("%#x", l.Func), fmt.Sprintf("%#x", l.Body), fmt.Sprint(l.Body % 64), fmt.Sprint(l.Body % 4096)}
	for _, op := range HotOps {
		if a, ok := l.Handlers[op]; ok {
			cols = append(cols, fmt.Sprint(a%64), fmt.Sprint(a%4096))
		} else {
			cols = append(cols, "-", "-")
		}
	}
	return strings.Join(cols, "\t")
}

// AppendLayout adds the row of bin to the layouts file of dir.
func AppendLayout(dir, bin, fn string, names []string) (Layout, error) {
	l, err := ReadLayout(bin, fn, names)
	if err != nil {
		return l, err
	}
	path := dir + "/layouts.tsv"
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return l, err
	}
	defer f.Close()
	if os.IsNotExist(statErr) {
		fmt.Fprintln(f, LayoutHeader())
	}
	_, err = fmt.Fprintln(f, l.Row(fileBase(bin)))
	return l, err
}

func fileBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// FirstEngineFunc returns the name of the function of internal/cengine that the
// binary places first in its text: the function a shift goes after, so that
// it moves the whole engine.
func FirstEngineFunc(bin string) (string, error) {
	f, err := elf.Open(bin)
	if err != nil {
		return "", err
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		return "", err
	}
	const pkg = "/internal/cengine."
	best, name := uint64(0), ""
	for _, s := range syms {
		i := strings.Index(s.Name, pkg)
		if i < 0 || !strings.HasSuffix(s.Name, ".abi0") || elf.ST_TYPE(s.Info) != elf.STT_FUNC {
			continue
		}
		if name == "" || s.Value < best {
			best, name = s.Value, strings.TrimSuffix(s.Name[i+len(pkg):], ".abi0")
		}
	}
	if name == "" {
		return "", fmt.Errorf("%s: no assembly function of internal/cengine", bin)
	}
	return name, nil
}

// bodyStart returns the offset of the body in the code of a function at addr:
// the first instruction after the NOPs that follow the PCALIGN $64 of its body.
// The Go assembler puts the frame set-up before that PCALIGN, so when the code
// does not start with NOPs the body starts after the next multiple of 64.
func bodyStart(addr uint64, code []byte) uint64 {
	if n := skipNops(code); n > 0 {
		return uint64(n)
	}
	a := (addr/64 + 1) * 64
	off := a - addr
	if off >= uint64(len(code)) {
		return 0
	}
	return off + uint64(skipNops(code[off:]))
}
