// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE-GO file.
//
// Changed by the liustack/quickjs Authors in 2026: doPrintf, doPrint and
// doPrintln of Go's fmt/print.go, and fmtS, fmtQ, fmtSbx, truncateString,
// padString and writePadding of fmt/format.go, and Int.Format of
// math/big/intconv.go, made to write the output of a call a piece at a
// time, with a read of vmhook.Stop between pieces, and to hand each verb on
// an argument that is not long to Go's fmt itself.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/hostmem"
	"github.com/liustack/quickjs/internal/vmhook"
)

// Go's fmt makes the whole output of a call before it returns, with no
// check of the VM: 16 MiB of %q took 22 ms on an Apple M4, and an output
// can be as long as a sixteenth of the room under the memory limit. So the
// functions here, which fmt.go calls for an output whose bound is more
// than shortOutput, read the format as Go's fmt does (doPrintf in
// fmt/print.go), and make the output a verb at a time: Go's fmt makes each
// verb of an argument that is not long, from a format of that verb alone,
// its flags, its width and its precision, and the text between verbs and
// the errors of the format are written here, as fmt writes them. A string,
// or an object as its json, longer than longArg, and a big int of more than
// longBits bits, are written here a part at a time, as fmt would write
// them. The call reads vmhook.Stop once at least every piece bytes of
// output, of a long argument, or of the format, every turnsPerStop verbs,
// and between two steps of the conversion of a long big int to decimal.
// TestPrintfAsGo and FuzzPrintfAsGo hold the output to Go's fmt byte for
// byte.

// piece is the most bytes of output, of a long argument or of the format a
// call works on between two reads of vmhook.Stop.
var piece = 1 << 16

// longArg is the longest string, or json of an object, that a verb hands Go's
// fmt whole. The tests set it to 0, so that every string goes the way of a
// long one.
var longArg = 1 << 16

// turnsPerStop is the most verbs a call makes between two reads of
// vmhook.Stop, as each verb takes a little time even when it writes
// nothing.
const turnsPerStop = 4096

// The texts of fmt/print.go.
const (
	commaSpaceString  = ", "
	nilAngleString    = "<nil>"
	percentBangString = "%!"
	missingString     = "(MISSING)"
	badIndexString    = "(BADINDEX)"
	extraString       = "%!(EXTRA "
	badWidthString    = "%!(BADWIDTH)"
	badPrecString     = "%!(BADPREC)"
	noVerbString      = "%!(NOVERB)"
	ldigits           = "0123456789abcdefx"
	udigits           = "0123456789ABCDEFX"
)

// flags are the flags, width and precision of a verb, as fmt's struct fmt
// holds them.
type flags struct {
	sharp, zero, plus, minus, space bool
	sharpV, plusV                   bool
	wid, prec                       int
	widPresent, precPresent         bool
}

// A printer makes the output of one call.
type printer struct {
	vm *quickjs.VM
	// buf is the output, and give gives it back (hostmem.Take).
	buf  []byte
	give func()
	// checked is the length of buf at the last read of vmhook.Stop, and
	// turns the verbs made since.
	checked, turns int
	// stopped is true once vmhook.Stop said the call is to end: the error
	// that ends it is pending, and the printer makes nothing more.
	stopped bool
	// quoted holds the quotes of one piece of a string, and args the
	// arguments of a verb for Go's fmt.
	quoted []byte
	args   []any
	// pows holds the powers of ten the conversion of a long big int to
	// decimal divides by.
	pows map[int]*big.Int
}

// stop reads vmhook.Stop, and reports whether the call is to end.
func (p *printer) stop() bool {
	p.checked, p.turns = len(p.buf), 0
	if !p.stopped && p.vm != nil && vmhook.Stop(p.vm) {
		p.stopped = true
	}
	return p.stopped
}

// turn counts a verb, and reads vmhook.Stop when a piece of output or
// turnsPerStop verbs have been made since the last read. It reports whether
// the call is to end.
func (p *printer) turn() bool {
	p.turns++
	if len(p.buf)-p.checked < piece && p.turns < turnsPerStop {
		return p.stopped
	}
	return p.stop()
}

// output is what p made, without a copy: p makes nothing more.
func (p *printer) output() string {
	if len(p.buf) == 0 {
		return ""
	}
	return unsafe.String(&p.buf[0], len(p.buf))
}

// write appends s, a piece at a time. It reports whether the call is to
// end.
func (p *printer) write(s string) bool {
	for len(s) > piece {
		p.buf = append(p.buf, s[:piece]...)
		s = s[piece:]
		if p.stop() {
			return true
		}
	}
	p.buf = append(p.buf, s...)
	return p.stopped
}

// sprintf is fmt.Sprintf(format, a...), and sprint and sprintln those of
// fmt.Sprint and fmt.Sprintln, made a piece at a time, in an output of n
// bytes, a bound on it, which never grows. The output is a block from 1 MiB
// (hostmem.Take), which give gives back: nothing may keep the string after
// that. Go clears an allocation in one go, with no read of vmhook.Stop,
// while the OS hands over its pages, and the pages of a block come as the
// output is written, a piece at a time. They are false, with no output and
// nothing to give back, when the call is to end.
func sprintf(vm *quickjs.VM, n int64, format string, a []any) (string, func(), bool) {
	p, ok := newPrinter(vm, n)
	if !ok {
		return "", func() {}, false
	}
	p.doPrintf(format, a)
	return p.result()
}

func sprint(vm *quickjs.VM, n int64, a []any) (string, func(), bool) {
	p, ok := newPrinter(vm, n)
	if !ok {
		return "", func() {}, false
	}
	p.doPrint(a)
	return p.result()
}

func sprintln(vm *quickjs.VM, n int64, a []any) (string, func(), bool) {
	p, ok := newPrinter(vm, n)
	if !ok {
		return "", func() {}, false
	}
	p.doPrintln(a)
	return p.result()
}

// newPrinter is a printer with an output of n bytes. It is false, with the
// out of memory error thrown, when the OS refuses the memory.
func newPrinter(vm *quickjs.VM, n int64) (*printer, bool) {
	if vm == nil || !hostmem.Mapped[byte](int(n)) {
		buf, give, err := hostmem.Take[byte](int(n))
		if err != nil {
			return nil, false
		}
		return &printer{vm: vm, buf: buf[:0], give: give}, true
	}
	p, give, ok := vmhook.Take(vm, int(n))
	if !ok {
		return nil, false
	}
	return &printer{vm: vm, buf: unsafe.Slice((*byte)(p), n)[:0], give: give}, true
}

// result is the output of p, the function that gives it back, and whether
// the call goes on. When it does not, the output is given back already.
func (p *printer) result() (string, func(), bool) {
	if p.stopped {
		p.give()
		return "", func() {}, false
	}
	return p.output(), p.give, true
}

// doPrintf is doPrintf of fmt/print.go.
func (p *printer) doPrintf(format string, a []any) {
	end := len(format)
	argNum := 0
	afterIndex := false
	reordered := false
formatLoop:
	for i := 0; i < end; {
		goodArgNum := true
		lasti := i
		for i < end && format[i] != '%' {
			if k := strings.IndexByte(format[i:min(end, i+piece)], '%'); k >= 0 {
				i += k
			} else if i = min(end, i+piece); i < end && p.stop() {
				return
			}
		}
		if i > lasti && p.write(format[lasti:i]) {
			return
		}
		if i >= end {
			break
		}

		i++
		var f flags
		flagsAt := i
	simpleFormat:
		for ; i < end; i++ {
			if (i-flagsAt)&(haltEvery-1) == haltEvery-1 && p.stop() {
				return
			}
			c := format[i]
			switch c {
			case '#':
				f.sharp = true
			case '0':
				f.zero = true
			case '+':
				f.plus = true
			case '-':
				f.minus = true
			case ' ':
				f.space = true
			default:
				if 'a' <= c && c <= 'z' && argNum < len(a) {
					spec := specFlags(format[flagsAt:i], f)
					if c == 'v' || c == 'w' {
						f.sharpV, f.sharp = f.sharp, false
						f.plusV, f.plus = f.plus, false
					}
					p.printArg(spec, nil, a[argNum], rune(c), f)
					argNum++
					i++
					if p.turn() {
						return
					}
					continue formatLoop
				}
				break simpleFormat
			}
		}
		// spec is the format of this verb alone, for Go's fmt: its flags,
		// then its width and its precision as given, numbers or '*', whose
		// arguments are in specArgs.
		spec := specFlags(format[flagsAt:i], f)
		var specArgs []any

		argNum, i, afterIndex = p.argNumber(argNum, format, i, len(a), &goodArgNum, &reordered)

		if i < end && format[i] == '*' {
			i++
			var arg any
			f.wid, f.widPresent, argNum, arg = argInt(a, argNum)
			if !f.widPresent {
				if p.write(badWidthString) {
					return
				}
			} else {
				spec += "*"
				specArgs = append(specArgs, arg)
			}
			if f.wid < 0 {
				f.wid = -f.wid
				f.minus = true
				f.zero = false
			}
			afterIndex = false
		} else {
			at := i
			f.wid, f.widPresent, i = p.parsenum(format, i, end)
			if p.stopped {
				return
			}
			if afterIndex && f.widPresent {
				goodArgNum = false
			}
			if f.widPresent {
				spec += specNumber(format[at:i], f.wid)
			}
		}

		if i+1 < end && format[i] == '.' {
			i++
			if afterIndex {
				goodArgNum = false
			}
			argNum, i, afterIndex = p.argNumber(argNum, format, i, len(a), &goodArgNum, &reordered)
			if i < end && format[i] == '*' {
				i++
				var arg any
				f.prec, f.precPresent, argNum, arg = argInt(a, argNum)
				if f.prec < 0 {
					f.prec = 0
					f.precPresent = false
				}
				if !f.precPresent {
					if p.write(badPrecString) {
						return
					}
				} else {
					spec += ".*"
					specArgs = append(specArgs, arg)
				}
				afterIndex = false
			} else {
				at := i
				f.prec, f.precPresent, i = p.parsenum(format, i, end)
				if p.stopped {
					return
				}
				if !f.precPresent {
					f.prec = 0
					f.precPresent = true
				}
				spec += "." + specNumber(format[at:i], f.prec)
			}
		}

		if !afterIndex {
			argNum, i, afterIndex = p.argNumber(argNum, format, i, len(a), &goodArgNum, &reordered)
		}
		if p.stopped {
			return
		}

		if i >= end {
			p.write(noVerbString)
			break
		}

		verb, size := utf8.DecodeRuneInString(format[i:])
		i += size

		switch {
		case verb == '%':
			p.buf = append(p.buf, '%')
		case !goodArgNum:
			p.badArgNum(verb)
		case argNum >= len(a):
			p.missingArg(verb)
		default:
			if verb == 'v' || verb == 'w' {
				f.sharpV, f.sharp = f.sharp, false
				f.plusV, f.plus = f.plus, false
			}
			p.printArg(spec, specArgs, a[argNum], verb, f)
			argNum++
		}
		if p.turn() {
			return
		}
	}

	if !reordered && argNum < len(a) {
		if p.write(extraString) {
			return
		}
		for i, arg := range a[argNum:] {
			if i > 0 {
				p.buf = append(p.buf, commaSpaceString...)
			}
			if arg == nil {
				p.buf = append(p.buf, nilAngleString...)
			} else {
				p.buf = append(p.buf, reflect.TypeOf(arg).String()...)
				p.buf = append(p.buf, '=')
				p.printArg("", nil, arg, 'v', flags{})
			}
			if p.turn() {
				return
			}
		}
		p.buf = append(p.buf, ')')
	}
}

// doPrint is doPrint of fmt/print.go.
func (p *printer) doPrint(a []any) {
	prevString := false
	for argNum, arg := range a {
		isString := arg != nil && reflect.TypeOf(arg).Kind() == reflect.String
		if argNum > 0 && !isString && !prevString {
			p.buf = append(p.buf, ' ')
		}
		p.printArg("", nil, arg, 'v', flags{})
		prevString = isString
		if p.turn() {
			return
		}
	}
}

// doPrintln is doPrintln of fmt/print.go.
func (p *printer) doPrintln(a []any) {
	for argNum, arg := range a {
		if argNum > 0 {
			p.buf = append(p.buf, ' ')
		}
		p.printArg("", nil, arg, 'v', flags{})
		if p.turn() {
			return
		}
	}
	p.buf = append(p.buf, '\n')
}

// argNumber is argNumber of fmt/print.go, which sets p.goodArgNum and
// p.reordered there.
func (p *printer) argNumber(argNum int, format string, i int, numArgs int, goodArgNum, reordered *bool) (newArgNum, newi int, found bool) {
	if len(format) <= i || format[i] != '[' {
		return argNum, i, false
	}
	*reordered = true
	index, wid, ok := parseArgNumberStop(format[i:], p.halt)
	if ok && 0 <= index && index < numArgs {
		return index, i + wid, true
	}
	*goodArgNum = false
	return argNum, i + wid, ok
}

func (p *printer) badArgNum(verb rune) {
	p.buf = append(p.buf, percentBangString...)
	p.buf = utf8.AppendRune(p.buf, verb)
	p.buf = append(p.buf, badIndexString...)
}

func (p *printer) missingArg(verb rune) {
	p.buf = append(p.buf, percentBangString...)
	p.buf = utf8.AppendRune(p.buf, verb)
	p.buf = append(p.buf, missingString...)
}

// argInt is intFromArg of fmt/print.go for the arguments the VM gives a Go
// function, which has every integer as an int, and also returns the
// argument it read, for the format of the verb alone.
func argInt(a []any, argNum int) (num int, isInt bool, newArgNum int, arg any) {
	newArgNum = argNum
	if argNum < len(a) {
		arg = a[argNum]
		num, isInt = arg.(int)
		newArgNum = argNum + 1
		if num > maxNumber || num < -maxNumber {
			num = 0
			isInt = false
		}
	}
	return
}

// halt is stop, for the parsers of a verb (parsenumStop).
func (p *printer) halt(int) bool { return p.stop() }

// parsenum is parsenum of fmt/print.go, which reads whether to stop in a
// long number (parsenumStop): a run of zeros may be as long as the format.
func (p *printer) parsenum(s string, start, end int) (num int, isnum bool, newi int) {
	if start >= end {
		return 0, false, end
	}
	return parsenumStop(s, start, end, p.halt)
}

// specFlags is the flags s of a verb, which f holds, for the format of the
// verb alone: s itself, or each flag of f once when s is longer, which Go's
// fmt reads the same way, as a flag sets a field however often it comes
// and wherever. A verb may have as many flags as the format has bytes.
func specFlags(s string, f flags) string {
	if len(s) <= len("#0+- ") {
		return s
	}
	b := make([]byte, 0, len("#0+- "))
	for _, x := range []struct {
		on bool
		c  byte
	}{{f.sharp, '#'}, {f.zero, '0'}, {f.plus, '+'}, {f.minus, '-'}, {f.space, ' '}} {
		if x.on {
			b = append(b, x.c)
		}
	}
	return string(b)
}

// specNumber is the digits s of a width or a precision, n, for the format of
// the verb alone: s itself, or n when s has leading zeros past what n
// takes, as a run of zeros may be as long as the format.
func specNumber(s string, n int) string {
	if len(s) <= 20 {
		return s
	}
	return strconv.Itoa(n)
}

// specialVerb reports whether verb, written after the flags of a format of
// one verb, would be read as a flag, a width, a precision or an index, as
// it is not where doPrintf found it. No type takes such a verb, and fmt
// writes it in the text of its error.
func specialVerb(verb rune) bool {
	switch verb {
	case '#', '0', '+', '-', ' ', '*', '.', '[':
		return true
	}
	return '1' <= verb && verb <= '9'
}

// stand is the verb a format of one verb takes in place of a special one:
// no type takes it either, and fmt writes it in the same place.
const stand = 'ä'

// printArg makes verb of arg with the flags f, which spec, with the
// arguments specArgs, writes for Go's fmt. A string, or the json of an
// object, longer than longArg, and a big int of more than longBits bits,
// are written here a part at a time. %T writes the name of their type, and
// %p of an object or a big int its address.
func (p *printer) printArg(spec string, specArgs []any, arg any, verb rune, f flags) {
	switch x := arg.(type) {
	case string:
		if verb != 'T' && len(x) > longArg {
			p.longString(x, verb, f)
			return
		}
	case *quickjs.Object:
		if verb != 'T' && verb != 'p' {
			if j := x.String(); len(j) > longArg {
				p.longObject(j, verb, f)
				return
			}
		}
	case *big.Int:
		if verb != 'T' && verb != 'p' && x.BitLen() > longBits {
			p.longBig(spec, specArgs, x, verb, f)
			return
		}
	}
	p.goVerb(spec, specArgs, arg, verb)
}

// goVerb is printArg by Go's fmt, from a format of the verb alone.
func (p *printer) goVerb(spec string, specArgs []any, arg any, verb rune) {
	v := verb
	if specialVerb(verb) {
		v = stand
	}
	var format string
	if spec == "" && v < utf8.RuneSelf {
		format = oneVerb[v]
	} else {
		format = "%" + spec + string(v)
	}
	at := len(p.buf)
	p.buf = p.appendf(p.buf, format, specArgs, arg)
	if v != verb {
		p.restoreVerb(at, verb)
	}
}

// appendf is fmt.Appendf(b, format, specArgs..., arg).
func (p *printer) appendf(b []byte, format string, specArgs []any, arg any) []byte {
	p.args = append(append(p.args[:0], specArgs...), arg)
	b = fmt.Appendf(b, format, p.args...)
	clear(p.args)
	return b
}

// oneVerb holds the format of each ASCII verb with no flags.
var oneVerb = func() (t [utf8.RuneSelf]string) {
	for c := range t {
		t[c] = "%" + string(rune(c))
	}
	return t
}()

// restoreVerb writes verb where Go's fmt wrote stand, in the text of the
// error it wrote from at: "%!ä(", which comes before any of the argument.
// An argument fmt writes the same for any verb, such as an empty struct,
// has no such text.
func (p *printer) restoreVerb(at int, verb rune) {
	i := bytes.Index(p.buf[at:], []byte(percentBangString+string(stand)))
	if i < 0 {
		return
	}
	i += at + len(percentBangString)
	rest := append([]byte(nil), p.buf[i+utf8.RuneLen(stand):]...)
	p.buf = append(utf8.AppendRune(p.buf[:i], verb), rest...)
}

// longString is printArg of a long string s, as fmtString of fmt/print.go.
func (p *printer) longString(s string, verb rune, f flags) {
	switch verb {
	case 'v':
		if f.sharpV {
			p.fmtQ(s, f)
		} else {
			p.fmtS(s, f)
		}
	case 's':
		p.fmtS(s, f)
	case 'x':
		p.fmtSx(s, f, ldigits)
	case 'X':
		p.fmtSx(s, f, udigits)
	case 'q':
		p.fmtQ(s, f)
	default:
		p.badVerb(verb, s, f)
	}
}

// badVerb is badVerb of fmt/print.go for a string s.
func (p *printer) badVerb(verb rune, s string, f flags) {
	p.buf = append(p.buf, percentBangString...)
	p.buf = utf8.AppendRune(p.buf, verb)
	p.buf = append(p.buf, "(string="...)
	if f.sharpV {
		p.fmtQ(s, f)
	} else {
		p.fmtS(s, f)
	}
	p.buf = append(p.buf, ')')
}

// longObject is printArg of an object whose json j is long: as a string for
// the verbs fmt takes its String method for, and otherwise as Go writes the
// struct that holds it.
func (p *printer) longObject(j string, verb rune, f flags) {
	switch {
	case verb == 'w':
		p.buf = append(p.buf, "%!w(*quickjs.Object=&"...)
		if f.sharpV {
			p.buf = append(p.buf, "quickjs.Object"...)
		}
		p.buf = append(p.buf, '{')
		if f.plusV || f.sharpV {
			p.buf = append(p.buf, "json:"...)
		}
		if f.sharpV {
			p.fmtQ(j, f)
		} else {
			p.fmtS(j, f)
		}
		p.buf = append(p.buf, "})"...)
	case verb == 'v' && f.sharpV:
		p.buf = append(p.buf, "&quickjs.Object{json:"...)
		p.fmtQ(j, f)
		p.buf = append(p.buf, '}')
	case verb == 'v', verb == 's', verb == 'x', verb == 'X', verb == 'q':
		p.longString(j, verb, f)
	default:
		p.buf = append(p.buf, "&{"...)
		p.badVerb(verb, j, flags{
			sharp: f.sharp, zero: f.zero, plus: f.plus, minus: f.minus, space: f.space,
			wid: f.wid, prec: f.prec, widPresent: f.widPresent, precPresent: f.precPresent,
		})
		p.buf = append(p.buf, '}')
	}
}

// cut is where the piece of s that starts at 0 ends, so that the pieces
// decode as s does (strconv decodes a string a character at a time, and an
// invalid byte as one): piece bytes in, or a byte or three before, at a
// byte that starts a character, which a decoding from the start of s always
// reaches. Without one in those four bytes, the bytes before piece hold no
// start of a character that takes in the byte at piece, which the decoding
// then reaches as well. piece is at least utf8.UTFMax.
func cut(s string) int {
	if len(s) <= piece {
		return len(s)
	}
	for k := piece; k > piece-utf8.UTFMax; k-- {
		if utf8.RuneStart(s[k]) {
			return k
		}
	}
	return piece
}

// truncate is truncateString of fmt/format.go.
func (p *printer) truncate(s string, f flags) string {
	if !f.precPresent {
		return s
	}
	n := f.prec
	turns := 0
	for i := range s {
		n--
		if n < 0 {
			return s[:i]
		}
		if turns++; turns == piece {
			turns = 0
			if p.stop() {
				return ""
			}
		}
	}
	return s
}

// runesUpTo is how many characters b has, counted up to n: no more than a
// width, a million, are counted.
func runesUpTo(b []byte, n int) int {
	k := 0
	for i := 0; i < len(b) && k < n; k++ {
		_, w := utf8.DecodeRune(b[i:])
		i += w
	}
	return k
}

// padding is writePadding of fmt/format.go.
func (p *printer) padding(n int, f flags) {
	if n <= 0 {
		return
	}
	c := byte(' ')
	if f.zero && !f.minus {
		c = '0'
	}
	for range n {
		p.buf = append(p.buf, c)
	}
}

// pad pads what p wrote from at to the width of f, as pad of fmt/format.go
// does: on its left, moving it, or on its right.
func (p *printer) pad(at int, f flags) {
	if !f.widPresent || f.wid == 0 {
		return
	}
	n := f.wid - runesUpTo(p.buf[at:], f.wid)
	if n <= 0 {
		return
	}
	if f.minus {
		p.padding(n, f)
		return
	}
	// What is padded has fewer characters than the width, a million, so
	// it is at most 4 MB.
	end := len(p.buf)
	p.padding(n, f)
	copy(p.buf[at+n:], p.buf[at:end])
	c := byte(' ')
	if f.zero {
		c = '0'
	}
	for i := at; i < at+n; i++ {
		p.buf[i] = c
	}
}

// fmtS is fmtS of fmt/format.go.
func (p *printer) fmtS(s string, f flags) {
	s = p.truncate(s, f)
	if p.stopped {
		return
	}
	n := 0
	if f.widPresent && f.wid != 0 {
		n = f.wid - runesUpTo(unsafe.Slice(unsafe.StringData(s), len(s)), f.wid)
	}
	if !f.minus {
		p.padding(n, f)
	}
	if p.write(s) {
		return
	}
	if f.minus {
		p.padding(n, f)
	}
}

// fmtQ is fmtQ of fmt/format.go: it quotes s a piece at a time.
func (p *printer) fmtQ(s string, f flags) {
	s = p.truncate(s, f)
	if p.stopped {
		return
	}
	at := len(p.buf)
	if f.sharp && p.canBackquote(s) {
		if p.stopped {
			return
		}
		p.buf = append(p.buf, '`')
		if p.write(s) {
			return
		}
		p.buf = append(p.buf, '`')
		p.pad(at, f)
		return
	}
	if p.stopped {
		return
	}
	p.buf = append(p.buf, '"')
	for len(s) > 0 {
		k := cut(s)
		if f.plus {
			p.quoted = strconv.AppendQuoteToASCII(p.quoted[:0], s[:k])
		} else {
			p.quoted = strconv.AppendQuote(p.quoted[:0], s[:k])
		}
		p.buf = append(p.buf, p.quoted[1:len(p.quoted)-1]...)
		s = s[k:]
		if len(s) > 0 && p.stop() {
			return
		}
	}
	p.buf = append(p.buf, '"')
	p.pad(at, f)
}

// canBackquote is strconv.CanBackquote, a piece at a time.
func (p *printer) canBackquote(s string) bool {
	for len(s) > 0 {
		k := cut(s)
		if !strconv.CanBackquote(s[:k]) {
			return false
		}
		s = s[k:]
		if len(s) > 0 && p.stop() {
			return false
		}
	}
	return true
}

// fmtSx is fmtSbx of fmt/format.go for a string.
func (p *printer) fmtSx(s string, f flags, digits string) {
	length := len(s)
	if f.precPresent && f.prec < length {
		length = f.prec
	}
	width := 2 * length
	if width > 0 {
		if f.space {
			if f.sharp {
				width *= 2
			}
			width += length - 1
		} else if f.sharp {
			width += 2
		}
	} else {
		if f.widPresent {
			p.padding(f.wid, f)
		}
		return
	}
	if f.widPresent && f.wid > width && !f.minus {
		p.padding(f.wid-width, f)
	}
	if f.sharp {
		p.buf = append(p.buf, '0', digits[16])
	}
	// Each byte takes 2 bytes, and one with a space before it 3, or 5
	// with '#'.
	per := 2
	if f.space {
		per = 3
		if f.sharp {
			per = 5
		}
	}
	for i := 0; i < length; {
		end := min(length, i+piece)
		at := len(p.buf)
		p.buf = slices.Grow(p.buf, per*(end-i))
		b := p.buf[at : at+per*(end-i)]
		k := 0
		for ; i < end; i++ {
			if f.space && i > 0 {
				b[k] = ' '
				k++
				if f.sharp {
					b[k], b[k+1] = '0', digits[16]
					k += 2
				}
			}
			c := s[i]
			b[k], b[k+1] = digits[c>>4], digits[c&0xF]
			k += 2
		}
		p.buf = p.buf[:at+k]
		if i < length && p.stop() {
			return
		}
	}
	if f.widPresent && f.wid > width && f.minus {
		p.padding(f.wid-width, f)
	}
}

// longBits is the most bits of a big int that Go's fmt writes in decimal
// whole, and that sprintf converts to decimal at once: math/big takes about
// 0.2 ms for 65,536 bits on an Apple M4, and 17 ms for the 1,040,000 bits of
// the longest the engine makes. The tests set it lower.
var longBits = 1 << 16

// longBig is printArg of a big int x of more than longBits bits. The verbs
// that write x in decimal, and a verb Int.Format (math/big/intconv.go) does
// not know, which writes x in decimal in the text of its error, are written
// here, with x converted a part at a time (decimal), and %w, which writes
// the words of x, is written some words at a time. Go's fmt writes the
// other verbs, in a radix that is a power of two, in time linear in the
// bits: 1,040,000 bits in binary in 0.45 ms on an Apple M4.
func (p *printer) longBig(spec string, specArgs []any, x *big.Int, verb rune, f flags) {
	switch verb {
	case 'b', 'o', 'O', 'x', 'X':
		p.goVerb(spec, specArgs, x, verb)
	case 'w':
		p.bigWords(spec, specArgs, x, f)
	case 'd', 's', 'v':
		p.bigFormat(x, f)
	default:
		p.buf = append(p.buf, percentBangString...)
		p.buf = utf8.AppendRune(p.buf, verb)
		p.buf = append(p.buf, "(big.Int="...)
		if x.Sign() < 0 {
			p.buf = append(p.buf, '-')
		}
		p.decimal(new(big.Int).Abs(x), 0)
		p.buf = append(p.buf, ')')
	}
}

// bigFormat is Int.Format of math/big/intconv.go for the decimal verbs, of
// a big int that is not 0.
func (p *printer) bigFormat(x *big.Int, f flags) {
	var sign byte
	switch {
	case x.Sign() < 0:
		sign = '-'
	case f.plus || f.plusV:
		sign = '+'
	case f.space:
		sign = ' '
	}
	at := len(p.buf)
	p.decimal(new(big.Int).Abs(x), 0)
	if p.stopped {
		return
	}
	n := len(p.buf) - at
	var left, zeros, right int
	if f.precPresent && n < f.prec {
		zeros = f.prec - n
	}
	length := zeros + n
	if sign != 0 {
		length++
	}
	if f.widPresent && length < f.wid {
		switch d := f.wid - length; {
		case f.minus:
			right = d
		case f.zero && !f.precPresent:
			zeros = d
		default:
			left = d
		}
	}
	// [left pad][sign][zero pad][digits][right pad]
	p.insert(at, zeros, '0')
	if sign != 0 {
		p.insert(at, 1, sign)
	}
	p.insert(at, left, ' ')
	for range right {
		p.buf = append(p.buf, ' ')
	}
}

// insert puts n bytes c in p.buf at at, moving what follows: digits of a
// big int, at most 315,653, and n is at most a width, a million.
func (p *printer) insert(at, n int, c byte) {
	if n <= 0 {
		return
	}
	end := len(p.buf)
	p.buf = slices.Grow(p.buf, n)[:end+n]
	copy(p.buf[at+n:], p.buf[at:end])
	for i := at; i < at+n; i++ {
		p.buf[i] = c
	}
}

// decimal appends the decimal digits of x, which is not negative, with
// zeros before them up to width digits. One of more than longBits bits is
// cut in two by a power of ten, and its two parts converted the same way,
// with a read of vmhook.Stop between two of these steps. The longest step is
// the first division of the longest big int, about 5 ms on an Apple M4,
// where math/big converts it at once in 17 ms.
func (p *printer) decimal(x *big.Int, width int) {
	if p.stopped {
		return
	}
	if x.BitLen() <= longBits {
		at := len(p.buf)
		p.buf = x.Append(p.buf, 10)
		p.insert(at, width-(len(p.buf)-at), '0')
		return
	}
	// x has d digits, or one fewer, and fewer than width when there is one.
	d := int(float64(x.BitLen())*math.Log10(2)) + 1
	if width > 0 {
		d = width
	}
	k := d / 2
	pow := p.pow10(k)
	if p.stop() {
		return
	}
	q, r := new(big.Int).QuoRem(x, pow, new(big.Int))
	if p.stop() {
		return
	}
	qw := 0
	if width > 0 {
		qw = width - k
	}
	p.decimal(q, qw)
	p.decimal(r, k)
}

// pow10 is 10 to the k, made once for a call.
func (p *printer) pow10(k int) *big.Int {
	if x, ok := p.pows[k]; ok {
		return x
	}
	if p.pows == nil {
		p.pows = map[int]*big.Int{}
	}
	x := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(k)), nil)
	p.pows[k] = x
	return x
}

// bigWords is %w of a big int x, which Go's fmt writes in the text of an
// error as the struct of x, a list of its words (badVerb in fmt/print.go):
// the text around the words is that of a big int of the sign of x with one
// word, and the words are written as Go's fmt writes those of a []big.Word,
// some at a time.
func (p *printer) bigWords(spec string, specArgs []any, x *big.Int, f flags) {
	format := "%" + spec + "w"
	one := big.NewInt(1)
	if x.Sign() < 0 {
		one.Neg(one)
	}
	head := p.appendf(nil, format, specArgs, one)
	elem := p.words(format, specArgs, one.Bits(), f)
	const tail = 3 // "]})" or "}})"
	p.buf = append(p.buf, head[:len(head)-tail-len(elem)]...)
	words := x.Bits()
	// Each word takes its width, its precision, 20 digits and a space or
	// two.
	n := max(1, piece/(max(f.wid, 0)+max(f.prec, 0)+24))
	for i := 0; i < len(words); i += n {
		if i > 0 {
			p.buf = append(p.buf, ' ')
			if f.sharpV {
				p.buf[len(p.buf)-1] = ','
				p.buf = append(p.buf, ' ')
			}
			if p.stop() {
				return
			}
		}
		p.buf = append(p.buf, p.words(format, specArgs, words[i:min(len(words), i+n)], f)...)
	}
	p.buf = append(p.buf, head[len(head)-tail:]...)
}

// words is what Go's fmt writes of the words w in format, a %w, between
// the brackets of the list.
func (p *printer) words(format string, specArgs []any, w []big.Word, f flags) []byte {
	p.quoted = p.appendf(p.quoted[:0], format, specArgs, w)
	open := len("%!w([]big.Word=[")
	if f.sharpV {
		open = len("%!w([]big.Word=[]big.Word{")
	}
	return p.quoted[open : len(p.quoted)-2]
}
