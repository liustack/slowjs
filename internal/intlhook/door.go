// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intlhook

import (
	"strconv"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The door is the only way a string of javascript comes into the Go of
// packages intl and temporal, and the only way one goes back
// (docs/changes/uncatchable.md, the door of Intl and Temporal).
//
// A Builtin opens a Door on its arguments and reads each string through it,
// and through nothing else (TestDoorIsTheOnlyWayIn in intl and temporal
// reads their source and fails on any other read of an argument). A short
// argument, of a Kind, is measured first where the engine keeps its length,
// and refused with a RangeError when it is longer than the cap of its kind,
// before any of it is read. Then the room under the memory limit is checked
// for what Go may take for all the arguments the call admitted so far, and
// only then is the argument read. Behind the door the work on short
// arguments is short: docs/changes/uncatchable.md gives the worst input
// measured at each cap.
//
// Long content, the text of a compare, a segmenter or a case mapping, the
// items of a list and a list of locales, has no cap. It is read with Long
// and LongWTF8, and its work is cut into pieces by a loop of package intl
// that checks the call between pieces.
//
// Every string and array a Builtin answers is made by Emit and the
// functions next to it.

// The longest string javascript may hand a Builtin of Intl or Temporal as
// each kind of short argument, in UTF-16 code units as javascript counts
// them.
const (
	// MaxTag is the longest locale tag, given alone: a list of locales is
	// long content.
	MaxTag = 16 << 10
	// MaxOption is the longest option, code, unit, key or id of Intl.
	MaxOption = 16 << 10
	// MaxTemporal is the longest string of Temporal: an ISO string, a
	// duration, a calendar or time zone id, an offset, and the strings of a
	// property bag.
	MaxTemporal = 16 << 10
	// MaxNumber is the longest numeric string Intl.NumberFormat reads.
	MaxNumber = 512 << 10
	// MaxDigits is the most digits a number may be written with once it is
	// rounded, the zeros a format writes out included. The longest BigInt, of
	// 315,653 digits, is under it with any options, and is the slowest to
	// write: its parts in Hindi with digits of four bytes take about 40 ms of
	// Go on an Apple M4, besides the 2.2 s the engine takes to make its digits.
	// A short string written with 525,312 digits takes 6.4 ms.
	MaxDigits = MaxNumber + 1<<10
)

// A Kind is the kind of a short argument, which sets its cap.
type Kind uint8

const (
	KindTag Kind = iota
	KindOption
	KindTemporal
	KindNumber
	// KindDigits is a number whose written form would be longer than
	// MaxDigits, which RefuseDigits refuses however short its string.
	KindDigits
	kinds
)

// caps holds, for each Kind, its cap, what its RangeError says, and the
// bytes Go may take for each code unit of an argument of that kind, which
// the door checks the room for before it reads it. At the cap, Go took at
// most 99 bytes a unit for a tag (Intl.Locale maximize), 129 for an option
// (DisplayNames of a language), 14 for a string of Temporal and 3 for a
// numeric string, with the call (TestRoomCoversGo in intl and temporal).
// The longest takes 1.5 ms for a tag, 3 ms for an option, 0.12 ms for a
// string of Temporal and 1.7 ms for a numeric string on an Apple M4.
var caps = [kinds]struct {
	max     int
	what    string
	perUnit int64
}{
	KindTag:      {MaxTag, "locale tag", 192},
	KindOption:   {MaxOption, "option", 192},
	KindTemporal: {MaxTemporal, "Temporal string", 32},
	KindNumber:   {MaxNumber, "numeric string", 8},
	KindDigits:   {MaxDigits, "number written", 0},
}

// RoomFixed is what Go may take for a call whatever its arguments: the
// formatter or the parse it makes, and the parts of its answer.
const RoomFixed = 64 << 10

// Max returns the cap of k.
func (k Kind) Max() int { return caps[k].max }

// Message is the message of the RangeError that refuses an argument of kind
// k longer than its cap.
func (k Kind) Message() string {
	if k == KindDigits {
		return "number would be written with more than " + strconv.Itoa(MaxDigits) + " digits"
	}
	return caps[k].what + " is longer than " + strconv.Itoa(caps[k].max) + " characters"
}

// tooLong is what the door panics with to refuse an argument, which
// NewBuiltin returns as a RangeError.
type tooLong struct{ kind Kind }

func (e tooLong) error() error { return &quickjs.Error{Name: "RangeError", Message: e.kind.Message()} }

// RefuseDigits ends the Builtin with the RangeError of a number whose
// written form would have more than MaxDigits digits, before it is
// formatted.
func RefuseDigits() { panic(tooLong{KindDigits}) }

// A Door reads the string arguments of one call of a Builtin given to
// NewBuiltin. It counts what Go may take for the arguments it admitted, so
// that the room is checked for all of them together.
type Door struct {
	a    quickjs.Args
	need int64
}

// Open returns the door of the call whose arguments are a.
func Open(a quickjs.Args) Door { return Door{a: a, need: RoomFixed} }

// VM is the VM of the call.
func (d *Door) VM() *quickjs.VM { return d.a.VM() }

// Short reads argument i, a short argument of kind k, as String does: a
// string longer than the cap of k ends the Builtin with a RangeError before
// any of it is read. ok is false for an argument that is not a string.
func (d *Door) Short(i int, k Kind) (string, bool) {
	if !d.admit(i, k) {
		return "", false
	}
	return String(d.a, i)
}

// ShortWTF8 is Short, read as WTF8 does.
func (d *Door) ShortWTF8(i int, k Kind) (string, bool) {
	if !d.admit(i, k) {
		return "", false
	}
	return WTF8(d.a, i)
}

// Long reads argument i, long content, as String does, whole.
func (d *Door) Long(i int) (string, bool) { return String(d.a, i) }

// LongWTF8 reads argument i, long content, as WTF8 does, whole.
func (d *Door) LongWTF8(i int) (string, bool) { return WTF8(d.a, i) }

// Len is the length of argument i, long content, in UTF-16 code units, read
// where the engine keeps it, so that a Builtin can count the room for what
// it does with the argument before it reads it. ok is false for an argument
// that is not a string.
func (d *Door) Len(i int) (int, bool) { return d.a.StringLen(i) }

// Piece is the most units of long content a Builtin works on between two
// checks of the call.
const Piece = 1 << 16

// LongUTF16 reads argument i, long content, as the UTF-16 code units
// javascript counts, lone surrogates as they are, into memory of Go's that
// the Builtin may keep after the call. It checks the room for them first, then
// copies them from where the engine keeps them, Piece units at a time, and
// checks the call between pieces: read as WTF-8 and turned back into UTF-16,
// a text of 8 Mi units held the eval timeout 19 to 40 ms on an Apple M4, with
// no check. ok is false for an argument that is not a string.
func (d *Door) LongUTF16(i int) ([]uint16, bool) {
	n, ok := d.a.StringLen(i)
	if !ok {
		return nil, false
	}
	d.Reserve(2 * int64(n))
	latin1, wide, release, ok := vmhook.Chars(d.a, i)
	if !ok {
		// A rope the engine could not join: the error is pending.
		panic(stopped{})
	}
	defer release()
	vm := d.a.VM()
	r := make([]uint16, max(len(latin1), len(wide)))
	for at := 0; at < len(r); at += Piece {
		Check(vm)
		end := min(at+Piece, len(r))
		if wide != nil {
			copy(r[at:end], wide[at:end])
			continue
		}
		for j, c := range latin1[at:end] {
			r[at+j] = uint16(c)
		}
	}
	return r, true
}

// Reserve counts n bytes more that Go may take for the call, such as those
// of a number written out, and ends the Builtin with the out of memory
// error when the room does not have them.
func (d *Door) Reserve(n int64) {
	d.need += n
	d.room()
}

// admit measures argument i, refuses it over the cap of k, and checks the
// room for it and the arguments admitted before. It is false for an
// argument that is not a string.
func (d *Door) admit(i int, k Kind) bool {
	n, ok := d.a.StringLen(i)
	if !ok {
		return false
	}
	if n > caps[k].max {
		panic(tooLong{k})
	}
	d.need += caps[k].perUnit * int64(n)
	d.room()
	return true
}

// room ends the Builtin with the out of memory error when the VM has no
// room for what the door counted.
func (d *Door) room() {
	vm := d.a.VM()
	if room := vmhook.Room(vm); room >= 0 && d.need > room {
		vmhook.OutOfMemory(vm)
		panic(stopped{})
	}
}
