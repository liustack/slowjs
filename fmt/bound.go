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
// the out of memory error when the bound is more than the VM has room for.
// The bound follows fmt's reading of the format (see doPrintf in
// fmt/print.go): which argument each verb takes, and what each verb can make
// of it.

// sizePrintf is a bound on the bytes of fmt.Sprintf(format, a...).
func sizePrintf(format string, a []any) int64 {
	n := int64(len(format))
	end := len(format)
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
			n += int64(wid) + int64(prec) + sizeArg(a[argNum], verb, sharp, space)
			argNum++
		}
	}
	if !reordered {
		for _, arg := range a[min(argNum, len(a)):] {
			n += 64 + sizeArg(arg, 'v', false, false)
		}
	}
	return n
}

// sizePrint is a bound on the bytes of fmt.Sprint(a...) and fmt.Sprintln(a...).
func sizePrint(a []any) int64 {
	n := int64(1)
	for _, arg := range a {
		n += 1 + sizeArg(arg, 'v', false, false)
	}
	return n
}

// sizeArg is a bound on the bytes verb makes of arg, with the flags sharp
// and space and no width or precision, for the arguments the VM gives a Go
// function (see quickjs.VM.Eval): a string, or an object as its json, comes
// out at most five times as long as it is, a number in at most a few
// hundred bytes. A verb that does not fit arg writes it as %v in the text of
// the error, in 64 bytes more.
func sizeArg(arg any, verb rune, sharp, space bool) int64 {
	switch arg := arg.(type) {
	case string:
		return sizeString(len(arg), verb, sharp, space)
	case *quickjs.Object:
		return sizeString(len(arg.String()), verb, sharp, space)
	case *big.Int:
		return int64(arg.BitLen()) + 64
	case float64:
		// %f of the largest float64 is 309 digits.
		return 384
	case int, bool, nil, quickjs.Undefined, quickjs.Unsupported:
		// %b of an int is 64 digits, and %#v of the structs their name.
		return 128
	}
	panic("fmt: internal error: an argument of a type the VM does not give")
}

// sizeString is sizeArg for a string of n bytes: %q writes a byte as up to
// four (\x80), %x as two, % x as three and %# x as five (" 0x80"), %#v as
// %q does, and any other verb as the string itself.
func sizeString(n int, verb rune, sharp, space bool) int64 {
	k := int64(1)
	switch {
	case verb == 'q', verb == 'v' && sharp:
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
