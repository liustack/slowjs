// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package fmt // import "github.com/liustack/quickjs/fmt"

import (
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/liustack/quickjs"
)

// Go's fmt makes the whole output of a call before it returns it or writes
// it, and a short format can ask for a long one: a width or a precision of
// up to about ten million bytes in each verb, and the same argument in many
// verbs. So the functions here find a bound on the bytes of the output from
// the format and the arguments, without making them, and end the call with
// the out of memory error when what Go may allocate to make that many bytes
// (allocated in fmt.go) is more than the VM has room for.
// The bound follows fmt's reading of the format (see doPrintf in
// fmt/print.go): which argument each verb takes, and what each verb can make
// of it.

// sizePrintf is a bound on the bytes of fmt.Sprintf(format, a...), and the
// format as fmt is to read it, which makes the same output in time linear
// in its length. It is false when that format does not fit in room bytes,
// which a room below 0 always does.
//
// fmt reads an index from a '[' to the first ']' after it, and a '[' with
// no ']' after it as an index that is not a number, which takes the '['
// alone. So fmt reads a format with many of these, such as "%[x" a million
// times, in time quadratic in its length: it looks for a ']' to the end
// after each. The format sizePrintf returns has "[]" for each of them with
// at least 3 bytes from it to the end, which fmt reads as the same index
// that is not a number, in the same place, so its output is the same.
// fmt takes such a '[' alone at most 3 times in a verb, and the verb has a
// byte more for each of them ('%', '.' and the verb itself), so there are
// at most half as many of them as bytes from the first to the end, which
// is all the format may grow by.
func sizePrintf(format string, a []any, room int64) (int64, string, bool) {
	n := int64(len(format))
	end := len(format)
	lastClose := strings.LastIndexByte(format, ']')
	var read strings.Builder
	at := 0
	fits := true
	argNum := 0
	afterIndex := false
	reordered := false
	// argNumber is pp.argNumber: the argument an index [n] at i names, or
	// argNum when there is none.
	argNumber := func(argNum, i int) (newArgNum, newi int, found, good bool) {
		if i >= end || format[i] != '[' {
			return argNum, i, false, true
		}
		reordered = true
		if i > lastClose {
			// parseArgNumber without its look for a ']'. It takes the '['
			// alone before fewer than 3 bytes, ']' or not, so "[]" stands
			// for "[" only before 3 or more.
			if end-i >= 3 && fits {
				if read.Cap() == 0 {
					size := len(format) + (end-i)/2 + 2
					if room >= 0 && int64(size) > room {
						fits = false
						return argNum, i + 1, false, false
					}
					read.Grow(size)
				}
				read.WriteString(format[at : i+1])
				read.WriteByte(']')
				at = i + 1
			}
			return argNum, i + 1, false, false
		}
		index, wid, ok := parseArgNumber(format[i:])
		if ok && 0 <= index && index < len(a) {
			return index, i + wid, true, true
		}
		return argNum, i + wid, ok, false
	}
	for i := 0; i < end; {
		good := true
		for i < end && format[i] != '%' {
			i++
		}
		if i >= end {
			break
		}
		i++

		var sharp, space bool
	flags:
		for ; i < end; i++ {
			switch format[i] {
			case '#':
				sharp = true
			case ' ':
				space = true
			case '0', '+', '-':
			default:
				break flags
			}
		}

		var ok bool
		argNum, i, afterIndex, ok = argNumber(argNum, i)
		good = good && ok
		wid, prec := 0, 0
		if i < end && format[i] == '*' {
			i++
			wid, argNum = intFromArg(a, argNum)
			wid = max(wid, -wid)
			afterIndex = false
		} else {
			var isnum bool
			wid, isnum, i = parsenum(format, i, end)
			if afterIndex && isnum {
				good = false
			}
		}
		if i+1 < end && format[i] == '.' {
			i++
			if afterIndex {
				good = false
			}
			argNum, i, afterIndex, ok = argNumber(argNum, i)
			good = good && ok
			if i < end && format[i] == '*' {
				i++
				prec, argNum = intFromArg(a, argNum)
				prec = max(prec, 0)
				afterIndex = false
			} else {
				prec, _, i = parsenum(format, i, end)
			}
		}
		if !afterIndex {
			argNum, i, afterIndex, ok = argNumber(argNum, i)
			good = good && ok
		}
		// The text of an error, such as %!(BADWIDTH) or %!v(MISSING),
		// before or in place of the value.
		n += 64
		if i >= end {
			break
		}

		verb, size := utf8.DecodeRuneInString(format[i:])
		i += size
		if verb != '%' && good && argNum < len(a) {
			n += sizeArg(a[argNum], verb, sharp, space, wid, prec)
			argNum++
		}
	}
	if !reordered {
		for _, arg := range a[min(argNum, len(a)):] {
			n += 64 + sizeArg(arg, 'v', false, false, 0, 0)
		}
	}
	if !fits {
		return 0, "", false
	}
	if read.Cap() == 0 {
		return n, format, true
	}
	read.WriteString(format[at:])
	return n, read.String(), true
}

// sizePrint is a bound on the bytes of fmt.Sprint(a...) and fmt.Sprintln(a...).
func sizePrint(a []any) int64 {
	n := int64(1)
	for _, arg := range a {
		n += 1 + sizeArg(arg, 'v', false, false, 0, 0)
	}
	return n
}

// sizeArg is a bound on the bytes verb makes of arg, with the flags sharp
// and space, the width wid and the precision prec, for the arguments the VM
// gives a Go function (see quickjs.VM.Eval): a string, or an object as its
// json, comes out at most five times as long as it is, a number in at most
// a few hundred bytes, and either padded to wid or prec once. A verb that
// does not fit arg writes it as %v in the text of the error, in 64 bytes
// more.
//
// %w, which only Errorf reads, writes arg in the text of the error as %v
// does with Go's reflection, which calls no String or Format method: an
// object as its struct, its json quoted for %#w as %#v quotes it, and a
// big.Int as its struct too, a list of words, each padded to wid and prec.
//
// %T writes the name of the type of arg, whatever arg holds, and %p of an
// object or a big int the address it is at in hex, so neither grows with
// arg: %T comes out in its width and a few bytes, as a precision only cuts
// the name, and %p in its width and precision and a few bytes. %p of any
// other type writes arg in the text of the error, as any verb that does
// not fit arg does.
func sizeArg(arg any, verb rune, sharp, space bool, wid, prec int) int64 {
	w := int64(wid) + int64(prec)
	switch verb {
	case 'T':
		// The longest name is that of quickjs.Unsupported, and a precision
		// only cuts it.
		return int64(wid) + 64
	case 'p':
		switch arg.(type) {
		case *quickjs.Object, *big.Int:
			// 0x and 16 digits.
			return w + 64
		}
	}
	switch arg := arg.(type) {
	case string:
		return w + sizeString(len(arg), verb, sharp, space)
	case *quickjs.Object:
		return w + sizeString(len(arg.String()), verb, sharp, space)
	case *big.Int:
		if verb == 'w' {
			// A word is at most 20 digits, or 18 for %#w (0xffffffffffffffff),
			// and ", " after it.
			return int64(len(arg.Bits())+2)*(w+24) + 64
		}
		return w + int64(arg.BitLen()) + 64
	case float64:
		// %f of the largest float64 is 309 digits.
		return w + 384
	case int, bool, nil, quickjs.Undefined, quickjs.Unsupported:
		// %b of an int is 64 digits, and %#v of the structs their name.
		return w + 128
	}
	panic("fmt: internal error: an argument of a type the VM does not give")
}

// sizeString is sizeArg for a string of n bytes: %q writes a byte as up to
// four (\x80), %x as two, % x as three and %# x as five (" 0x80"), %#v and
// %#w as %q does, and any other verb as the string itself.
func sizeString(n int, verb rune, sharp, space bool) int64 {
	k := int64(1)
	switch {
	case verb == 'q', (verb == 'v' || verb == 'w') && sharp:
		k = 4
	case (verb == 'x' || verb == 'X') && space && sharp:
		k = 5
	case (verb == 'x' || verb == 'X') && space:
		k = 3
	case verb == 'x' || verb == 'X':
		k = 2
	}
	return k*int64(n) + 64
}

// The functions below read the format as fmt does.

// maxNumber is the largest width, precision or argument index fmt takes in
// a format, and the largest a '*' takes from an argument, either way.
const maxNumber = 1_000_000

// parsenum reads the decimal number at s[i:end], and returns it, whether
// there was one, and where it ends. fmt stops at the digit after one that
// makes it larger than maxNumber, and then reads no more of the format: it
// returns end.
func parsenum(s string, i, end int) (num int, isnum bool, next int) {
	for next = i; next < end && s[next] >= '0' && s[next] <= '9'; next++ {
		if num > maxNumber {
			return 0, false, end
		}
		num = 10*num + int(s[next]-'0')
		isnum = true
	}
	return num, isnum, next
}

// parseArgNumber reads the index at the start of s, which is a '[': the
// argument it names, counted from 0, how many bytes it takes, as far as its
// ']', and whether it is a number. Without a ']' it takes the '[' alone.
func parseArgNumber(s string) (index, size int, ok bool) {
	if len(s) < 3 {
		return 0, 1, false
	}
	j := strings.IndexByte(s[1:], ']')
	if j < 0 {
		return 0, 1, false
	}
	j++
	n, isnum, next := parsenum(s, 1, j)
	if !isnum || next != j {
		return 0, j + 1, false
	}
	return n - 1, j + 1, true
}

// intFromArg is the width or precision a '*' takes from argument argNum, and
// the argument after it: the argument when it is an int of at most
// maxNumber either way, which is how the VM gives a Go function every
// integer, and 0 for any other.
func intFromArg(a []any, argNum int) (num, next int) {
	if argNum >= len(a) {
		return 0, argNum
	}
	if n, ok := a[argNum].(int); ok && n >= -maxNumber && n <= maxNumber {
		num = n
	}
	return num, argNum + 1
}
