// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bufio"
	"bytes"
	"cmp"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
// names, the handler of each of HotOps from its jump table (see jumpTable).
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
	base, ok := jumpTable(s.Value, code, func(a uint64) []byte {
		if a < text.Addr || a+256*4 > text.Addr+uint64(len(data)) {
			return nil
		}
		return at(a)[:256*4]
	})
	if !ok {
		return l, fmt.Errorf("%s: no jump table of %s: no address that a lea relative to the instruction pointer loads holds 256 offsets into the function", bin, fn)
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
	cols := []string{"binary", "sha256", "func", "body", "body%64", "body%4096"}
	for _, op := range HotOps {
		cols = append(cols, op+"%64", op+"%4096")
	}
	return strings.Join(cols, "\t")
}

// Row is the line of the layouts file of one binary, with the SHA-256 of the
// file.
func (l Layout) Row(name, hash string) string {
	cols := []string{name, hash, fmt.Sprintf("%#x", l.Func), fmt.Sprintf("%#x", l.Body), fmt.Sprint(l.Body % 64), fmt.Sprint(l.Body % 4096)}
	for _, op := range HotOps {
		if a, ok := l.Handlers[op]; ok {
			cols = append(cols, fmt.Sprint(a%64), fmt.Sprint(a%4096))
		} else {
			cols = append(cols, "-", "-")
		}
	}
	return strings.Join(cols, "\t")
}

// WriteLayout puts the row of the binary name in the layouts file of dir, in
// place of the row it had: a binary that is built again has one row, of the
// file now there.
func WriteLayout(dir, name, hash string, l Layout) error {
	path := filepath.Join(dir, "layouts.tsv")
	lines := []string{LayoutHeader()}
	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return err
	default:
		old := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		if old[0] != LayoutHeader() {
			return fmt.Errorf("%s has other columns: move it away", path)
		}
		for _, line := range old[1:] {
			if n, _, _ := strings.Cut(line, "\t"); n != name {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, l.Row(name, hash))
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// CheckLayout reports how the interpreter of a binary at p did not land where
// p puts it, against the formal binary of the same side. The first function
// after the shift that is aligned to 64 bytes takes up some of it in the
// padding in front of it, and the interpreter moves with that function. So it
// moves by a multiple of 64 within 32 bytes of the shift: the shift rounded up
// or down to 64, whichever the padding of the formal binary allows (a shift
// that is a multiple of 64 always moves it exactly). Nothing moves when the
// padding takes up all of it. The body and every hot handler sit front bytes
// later in the interpreter, so they are at the formal places plus front,
// modulo 64.
func CheckLayout(formal, l Layout, p Position) error {
	d := int64(l.Func) - int64(formal.Func)
	switch {
	case p.Shift == 0 && d != 0:
		return fmt.Errorf("position %s: the interpreter moved by %d bytes without a shift", p, d)
	case p.Shift > 0 && d == 0:
		return fmt.Errorf("position %s: the interpreter did not move, the shift of %d was taken up by the alignment and the layout is the formal one for the code after it", p, p.Shift)
	case p.Shift > 0 && (d%64 != 0 || d < int64(p.Shift)-32 || d > int64(p.Shift)+32):
		return fmt.Errorf("position %s: the interpreter moved by %d bytes, not a multiple of 64 within 32 of the shift of %d", p, d, p.Shift)
	}
	at := func(name string, a, f uint64) error {
		if got, want := int64(a)-int64(l.Func), int64(f)-int64(formal.Func)+int64(p.Front); got != want {
			return fmt.Errorf("position %s: %s is %d bytes into the interpreter, want %d (formal plus %d)", p, name, got, want, p.Front)
		}
		return nil
	}
	if err := at("the body", l.Body, formal.Body); err != nil {
		return err
	}
	for op, f := range formal.Handlers {
		a, ok := l.Handlers[op]
		if !ok {
			return fmt.Errorf("position %s: no handler of %s", p, op)
		}
		if err := at("the handler of "+op, a, f); err != nil {
			return err
		}
	}
	return nil
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

// jumpTable finds the jump table of the interpreter whose code is at addr: 256
// offsets of 4 bytes, each relative to the table, that all land in the
// function. The dispatch loads the address of the table with a lea relative to
// the instruction pointer, into whichever register the compiler chose, so every
// such lea is a candidate: the table is the most common target that holds 256
// offsets into the function. read gives the 1024 bytes at an address, or nil
// when they are not all in the text.
func jumpTable(addr uint64, code []byte, read func(a uint64) []byte) (uint64, bool) {
	count := map[uint64]int{}
	for i := 0; i+7 <= len(code); i++ {
		// REX.W (any of R, X, B), opcode 8d, ModRM with mod 00 and r/m 101:
		// lea disp32(%rip), reg
		if code[i]&0xf8 == 0x48 && code[i+1] == 0x8d && code[i+2]&0xc7 == 0x05 {
			disp := int32(binary.LittleEndian.Uint32(code[i+3:]))
			count[uint64(int64(addr)+int64(i)+7+int64(disp))]++
		}
	}
	targets := make([]uint64, 0, len(count))
	for t := range count {
		targets = append(targets, t)
	}
	slices.SortFunc(targets, func(a, b uint64) int {
		if c := cmp.Compare(count[b], count[a]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	end := addr + uint64(len(code))
	for _, t := range targets {
		tb := read(t)
		if tb == nil {
			continue
		}
		ok := true
		for i := 0; i < 256 && ok; i++ {
			h := uint64(int64(t) + int64(int32(binary.LittleEndian.Uint32(tb[i*4:]))))
			ok = h >= addr && h < end
		}
		if ok {
			return t, true
		}
	}
	return 0, false
}
