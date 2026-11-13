// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/vmhook"
	"github.com/liustack/quickjs/internal/whatwgurl"
)

// The door is the only way a string of javascript comes into Go, and the only
// way one goes back (see docs/changes/url.md, the limits).
//
// A Builtin of this package reads its string arguments with admit, and with
// nothing else (TestDoorIsTheOnlyWayIn reads the source and fails on any other
// read of an argument). admit reads the length of each where the engine keeps
// it, refuses the call when one is longer than whatwgurl.MaxUnits, checks once
// the room under the memory limit for what Go may take for all of them, and
// only then reads them, into whatwgurl.Inputs, which nothing else makes. Behind
// it nothing is long: the parser works in proportion to its input, but for the
// IDNA of a host, which whatwgurl.MaxDomainUnits bounds, so a call is over in a
// few milliseconds of Go, with no look at the stop flag inside it. Each call
// looks once as it starts (newBuiltin in bridge.go).
//
// Every string a Builtin answers is made by emit, which the package never
// calls with one longer than MaxUnits: a URL whose serialization would be, or
// an encoding that would be, is refused by whatwgurl. So what comes out of the
// door can always go back in.

// roomPerUnit, roomPerDomain and roomFixed are the most Go allocates for a
// call: roomPerUnit bytes for each UTF-16 unit of the strings it admits,
// roomPerDomain for each host it may parse, and roomFixed whatever they are.
// The IDNA of a domain costs the most for each unit, and a code point can map
// to 18 (idna.Widest), so a short string can make a domain of
// whatwgurl.MaxDomainUnits once mapped, which is as long as one may be: its
// room is not in proportion to the string but the most a domain can take.
// TestRoomCoversGo measures what Go allocates, in all, on every call and every
// shape of input, those of the code points the tables grow the most among
// them, and fails when Go took more. What Go holds at once, which is what the
// room is for, is less.
const (
	roomPerUnit   = 160
	roomPerDomain = 128 << 10
	roomFixed     = 16 << 10
)

// admitHook, when a test sets it, is called with the room admit is about to
// check.
var admitHook func(need int64)

// A verdict is what admit makes of a call.
type verdict int

const (
	// admitted: the strings are read.
	admitted verdict = iota
	// tooLong: a string is longer than MaxUnits, and nothing was read.
	tooLong
	// ended: the call is to end, with an error that is pending (there was no
	// room, or a string could not be read).
	ended
	// notParts: the parts of a URL record are longer in all than MaxUnits,
	// which those of a URL never are, and nothing was read.
	notParts
)

// maxStrings is the most strings a call admits: the value of a setter and the
// eight parts of the record it changes.
const maxStrings = 9

// admit reads the string arguments of a at idx, after the checks above, into
// in, in the order of idx. An argument that is not a string, such as undefined
// for no base, is the empty Input and false in present. domains is how many
// hosts the call may parse from them, at most one a string. The last parts
// strings of idx are the parts of one URL record, which sum to at most its
// href and so to at most MaxUnits: admit checks it, as they come from
// javascript, and refuses a call whose parts sum to more.
func admit(a quickjs.Args, domains, parts int, idx ...int) (in [maxStrings]whatwgurl.Input, present [maxStrings]bool, v verdict) {
	units, record := 0, 0
	for k, i := range idx {
		n, ok := a.StringLen(i)
		if !ok {
			continue
		}
		if n > whatwgurl.MaxUnits {
			return in, present, tooLong
		}
		units += n
		if k >= len(idx)-parts {
			record += n
		}
		present[k] = true
	}
	if record > whatwgurl.MaxUnits {
		return in, present, notParts
	}
	if n := count(present); domains > n {
		// No host in a string that is not there.
		domains = n
	}
	need := int64(roomPerUnit)*int64(units) + int64(roomPerDomain)*int64(domains) + roomFixed
	if h := admitHook; h != nil {
		h(need)
	}
	vm := a.VM()
	if room := vmhook.Room(vm); room >= 0 && need > room {
		vmhook.OutOfMemory(vm)
		return in, present, ended
	}
	for k, i := range idx {
		if !present[k] {
			continue
		}
		// A rope is joined here, by the engine, into a string of at most MaxUnits.
		// intlhook.String ends the Builtin when that fails.
		s, _ := intlhook.String(a, i)
		x, ok := whatwgurl.NewInput(s)
		if !ok {
			panic("url: a string the engine said was short is long")
		}
		in[k] = x
	}
	return in, present, admitted
}

// count is how many of present are true.
func count(present [maxStrings]bool) int {
	n := 0
	for _, p := range present {
		if p {
			n++
		}
	}
	return n
}

// emit is the javascript string of s, which is at most MaxUnits long. Every
// string a Builtin answers is made here.
func emit(vm *quickjs.VM, s string) (quickjs.Value, error) {
	if _, ok := whatwgurl.NewInput(s); !ok {
		panic("url: a result longer than whatwgurl.MaxUnits")
	}
	return vm.NewString(s)
}
