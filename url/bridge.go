// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package url

import (
	"strings"
	"sync"

	"github.com/liustack/tifajs"
	"github.com/liustack/tifajs/internal/intlhook"
	"github.com/liustack/tifajs/internal/whatwgurl"
)

// The Builtins url.js is called with. A URL is javascript's, in the VM, under
// its memory limit, and nothing a script gave is held in Go when a call ends,
// but for the last base a VM parsed and the last record it made (keptBase and
// keptRecord below). Each reads its strings
// through the door (door.go) and answers undefined when the call is to end:
// the uncatchable error is then pending and the answer is not read.

// The bits of the record that parts puts after the parts and set takes, which
// the getters do not show, as url.js reads them: the URL has a host (which may
// be empty), its path is opaque, it has a query, it has a fragment (each of
// which may be empty).
const (
	hasHost = 1 << iota
	opaque
	hasQuery
	hasFragment
)

// The setter parts of set, as url.js numbers them.
const (
	setProtocol = 1 + iota
	setUsername
	setPassword
	setHost
	setHostname
	setPort
	setPathname
	setSearch
	setHash
)

// maxBaseBytes is the longest base a VM keeps parsed, in bytes of UTF-8, and
// the longest href of that base.
const maxBaseBytes = 128

// What a VM keeps, at most: the base and its URL, the href of the record
// (which its parts are cut from) and its URL, and its vmState with its entry
// in states. The text of a URL's strings and path is at most its href. Each
// is in a buffer of at most twice its length and 64 bytes (whatwgurl copies a
// part the parser takes and a path ".." made shorter from a buffer with more
// room than that, and append grows the others less), rounded up to a size
// class of Go's allocator, which adds at most an eighth and 16 bytes at these
// sizes. So a URL with an href of H bytes keeps at most 2.25H + 328 bytes,
// and 192 for the URL: 808 for the base and 1,672 for the record. With the 128
// bytes of the base, the 512 of the href, and under 400 for the state and its
// entry, a VM keeps at most 3.5 KiB (TestKeptIsBounded checks keptMost).
const keptMost = 4096

// keptBase is the last base a VM parsed whose string and href are at most
// maxBaseBytes, and its parse: a router resolves every link of a page against one base, so
// most calls parse it no more. Each VM has its own (states), which the
// Builtins change in the calls of that VM, one at a time, and which goes when
// the VM closes: at most maxBaseBytes and the URL made of them. The parser
// only reads a base and copies what it takes of it, so a parsed base gives
// what a new parse of it would.
type keptBase struct {
	s string
	u *whatwgurl.URL
}

// baseParseHook, when a test sets it, is called for each base parsed.
var baseParseHook func()

// parse parses in as a base, or returns the one kept when it is the same.
func (b *keptBase) parse(in whatwgurl.Input) (*whatwgurl.URL, error) {
	s := in.Text()
	if b.u != nil && b.s == s {
		return b.u, nil
	}
	if h := baseParseHook; h != nil {
		h()
	}
	u, err := whatwgurl.Parse(in, nil)
	if err != nil || len(s) > maxBaseBytes {
		return u, err
	}
	// percent-encoding and IDNA can make an href longer than its string
	if href, _ := u.Serialize(); len(href) <= maxBaseBytes {
		b.s, b.u = strings.Clone(s), u
	}
	return u, nil
}

// parseArgs parses argument 0 against argument 1, a base or undefined. ok is
// false when the input is not a URL, a string is longer than MaxUnits, the URL
// would be, or the call is to end.
func parseArgs(a tifajs.Args, kept *keptBase) (*whatwgurl.URL, bool) {
	in, present, v := admit(a, 2, 0, 0, 1)
	if v != admitted || !present[0] {
		return nil, false
	}
	var base *whatwgurl.URL
	if present[1] {
		b, err := kept.parse(in[1])
		if err != nil {
			return nil, false
		}
		base = b
	}
	u, err := whatwgurl.Parse(in[0], base)
	return u, err == nil
}

// maxKeptBytes is the longest href of a record a VM keeps (keptRecord).
const maxKeptBytes = 512

// keptRecord is the last record parts made for a VM, when its href is at most
// maxKeptBytes: its parts and bits, as url.js hands them back to set, and the
// URL they are the parts of. A router changes a URL just after it made it
// (u.pathname = '/zh-CN' + u.pathname), and set then takes a copy of that URL
// in place of making the record again from its parts (FromParts), which
// parses its href once more. Parts and bits equal to these are the parts of
// that URL, from which FromParts makes a URL that gives them back: the same
// record. Nothing changes u: set changes a copy. A URL holds about the bytes
// it is made of, however long the input was (whatwgurl's take and Path.Pop),
// so this is at most maxKeptBytes of href and the URL made of it (keptMost),
// which goes when the VM closes.
type keptRecord struct {
	p whatwgurl.Parts
	u *whatwgurl.URL
}

// vmState is what the Builtins keep for one VM between its calls, which the
// VM makes one at a time.
type vmState struct {
	base keptBase
	last keptRecord
}

// states holds the vmState of each VM that Register set up, until the VM
// closes (Register gives it to OnClose). The Builtins are the package's, made
// once, as a process may have only 32768 of them, so a VM's state is found
// by the VM that calls.
var states sync.Map // *tifajs.VM -> *vmState

// stateOf returns the state of vm. Only url.js calls the Builtins, and only
// in a VM that Register set up, which has one.
func stateOf(vm *tifajs.VM) *vmState {
	st, ok := states.Load(vm)
	if !ok {
		panic("url: a Builtin of package url called in a VM that Register did not set up")
	}
	return st.(*vmState)
}

// fromPartsHook, when a test sets it, is called for each record set makes
// again from its parts.
var fromPartsHook func()

// parts is the array url.js keeps for a URL: the href, then protocol,
// username, password, host, hostname, port, pathname, search, hash and
// origin, and last the bits of the record the getters do not show, which with
// the parts make the record again (record). It keeps the record in st.last,
// when its href is at most maxKeptBytes, and nothing there otherwise.
func parts(vm *tifajs.VM, u *whatwgurl.URL, st *vmState) (tifajs.Value, error) {
	p, href := u.Parts()
	st.last = keptRecord{}
	if len(href) <= maxKeptBytes {
		st.last = keptRecord{p: p, u: u}
	}
	host := p.Hostname.Text()
	if port := p.Port.Text(); port != "" {
		host += ":" + port
	}
	strs := [...]string{
		href,
		p.Protocol.Text(),
		p.Username.Text(),
		p.Password.Text(),
		host,
		p.Hostname.Text(),
		p.Port.Text(),
		p.Pathname.Text(),
		p.Search.Text(),
		p.Hash.Text(),
		u.Origin(),
	}
	flags := 0
	if p.HasHost {
		flags |= hasHost
	}
	if p.Opaque {
		flags |= opaque
	}
	if p.HasQuery {
		flags |= hasQuery
	}
	if p.HasFragment {
		flags |= hasFragment
	}
	// Each string is made once: a URL has several empty parts, and its host is
	// its hostname when it has no port. first[i] is where strs[i] is first.
	var vals [len(strs) + 1]tifajs.Value
	var first [len(strs)]int
	free := func(n int) {
		for i := range n {
			if first[i] == i {
				vals[i].Free()
			}
		}
	}
	for i, s := range strs {
		first[i] = i
		for j := range i {
			if strs[j] == s {
				first[i] = j
				break
			}
		}
		if first[i] != i {
			vals[i] = vals[first[i]]
			continue
		}
		v, err := emit(vm, s)
		if err != nil {
			free(i)
			return tifajs.Value{}, err
		}
		vals[i] = v
	}
	vals[len(strs)] = vm.NewInt(flags)
	arr, err := vm.NewArray(vals[:]...)
	free(len(strs))
	return arr, err
}

// parseBuiltin is parse(input, base): the parts of the URL, or undefined.
var parseBuiltin = newBuiltin("parse", 2, func(a tifajs.Args) (tifajs.Value, error) {
	st := stateOf(a.VM())
	u, ok := parseArgs(a, &st.base)
	if !ok {
		return tifajs.UndefinedValue, nil
	}
	return parts(a.VM(), u, st)
})

// canParseBuiltin is canParse(input, base): 1 or 0.
var canParseBuiltin = newBuiltin("canParse", 2, func(a tifajs.Args) (tifajs.Value, error) {
	if _, ok := parseArgs(a, &stateOf(a.VM()).base); ok {
		return a.VM().NewInt(1), nil
	}
	return a.VM().NewInt(0), nil
})

// errNotParts is the error of set for arguments that url.js does not hand it:
// a part that is not a setter's, or a record that is not a URL's. A script
// cannot reach set (see docs/internals/changes/url.md), so only a fault of url.js could.
var errNotParts = &tifajs.Error{Name: "TypeError", Message: "URL: the record of the URL is not that of a URL"}

// setBuiltin is set(part, value, flags, protocol, username, password,
// hostname, port, pathname, search, hash): the setter of a part on the URL
// whose record are the parts that follow the value, with the bits of parts,
// as the parts of the URL it makes. It answers 0 when the value is longer
// than MaxUnits or the URL would be, and undefined when the call is to end.
// Parts and bits that are not those of a URL, and a part number that is not
// one of the setters, are a TypeError: the URL is not changed.
var setBuiltin = newBuiltin("set", 11, set)

// set is the work of setBuiltin.
func set(a tifajs.Args) (tifajs.Value, error) {
	vm := a.VM()
	st := stateOf(vm)
	pn, ok := a.Number(0)
	part := int(pn)
	if !ok || float64(part) != pn || part < setProtocol || part > setHash {
		return tifajs.Value{}, errNotParts
	}
	fn, ok := a.Number(2)
	f := int(fn)
	if !ok || float64(f) != fn || f&^(hasHost|opaque|hasQuery|hasFragment) != 0 {
		return tifajs.Value{}, errNotParts
	}
	// FromParts parses the hostname of the record again, which IDNA may take
	// when it holds a percent-encoded code point, and a setter of the host
	// parses its value.
	domains := 1
	if part == setHost || part == setHostname {
		domains = 2
	}
	in, present, v := admit(a, domains, 8, 1, 3, 4, 5, 6, 7, 8, 9, 10)
	switch {
	case v == tooLong:
		return vm.NewInt(0), nil
	case v == notParts:
		return tifajs.Value{}, errNotParts
	case v != admitted:
		return tifajs.UndefinedValue, nil
	}
	for _, ok := range present[:9] {
		if !ok {
			return tifajs.Value{}, errNotParts
		}
	}
	p := whatwgurl.Parts{
		Protocol: in[1], Username: in[2], Password: in[3], Hostname: in[4],
		Port: in[5], Pathname: in[6], Search: in[7], Hash: in[8],
		HasHost: f&hasHost != 0, Opaque: f&opaque != 0, HasQuery: f&hasQuery != 0, HasFragment: f&hasFragment != 0,
	}
	var u *whatwgurl.URL
	if st.last.u != nil && st.last.p == p {
		u = st.last.u.Clone()
	} else {
		if h := fromPartsHook; h != nil {
			h()
		}
		var err error
		if u, err = whatwgurl.FromParts(p); err != nil {
			return tifajs.Value{}, errNotParts
		}
	}
	var err error
	switch value := in[0]; part {
	case setProtocol:
		err = u.SetProtocol(value)
	case setUsername:
		err = u.SetUsername(value)
	case setPassword:
		err = u.SetPassword(value)
	case setHost:
		err = u.SetHost(value)
	case setHostname:
		err = u.SetHostname(value)
	case setPort:
		err = u.SetPort(value)
	case setPathname:
		err = u.SetPathname(value)
	case setSearch:
		err = u.SetSearch(value)
	case setHash:
		err = u.SetHash(value)
	}
	if err != nil {
		return vm.NewInt(0), nil
	}
	return parts(vm, u, st)
}

// errTooLong is the error of a name or a value of a query string, or of its
// encoding, that is longer than MaxUnits.
var errTooLong = &tifajs.Error{Name: "RangeError", Message: "URLSearchParams: a name or a value, or its encoding, is longer than 65536 UTF-16 code units"}

// formDecodeBuiltin is formDecode(field): a name or a value of a query string
// as the application/x-www-form-urlencoded parser makes it. url.js splits the
// string at & and = and calls it for a field that has a + or a %.
var formDecodeBuiltin = newBuiltin("formDecode", 1, func(a tifajs.Args) (tifajs.Value, error) {
	in, present, v := admit(a, 0, 0, 0)
	switch {
	case v == tooLong:
		return tifajs.Value{}, errTooLong
	case v != admitted || !present[0]:
		return tifajs.UndefinedValue, nil
	}
	return emit(a.VM(), whatwgurl.FormDecode(in[0]))
})

// formEncodeBuiltin is formEncode(field): a name or a value as the
// application/x-www-form-urlencoded serializer writes it.
var formEncodeBuiltin = newBuiltin("formEncode", 1, func(a tifajs.Args) (tifajs.Value, error) {
	in, present, v := admit(a, 0, 0, 0)
	switch {
	case v == tooLong:
		return tifajs.Value{}, errTooLong
	case v != admitted || !present[0]:
		return tifajs.UndefinedValue, nil
	}
	out, err := whatwgurl.FormEncode(in[0])
	if err != nil {
		return tifajs.Value{}, errTooLong
	}
	return emit(a.VM(), out)
})

// bridge is the list of Builtins url.js is a function of, in its order.
var bridge = []*tifajs.Builtin{parseBuiltin, setBuiltin, formDecodeBuiltin, formEncodeBuiltin, canParseBuiltin}

// callHook, when a test sets it, is called as each Builtin of this package
// starts (enter true) and as it ends.
var callHook func(enter bool)

// newBuiltin is intlhook.NewBuiltin with callHook, and one look at the stop
// flag as each call starts. The engine counts toward its next poll the
// bytecode it runs and not the time a Go function takes, so a loop that calls
// a Builtin taking a few milliseconds would make many such calls between two
// polls: the look ends the script at the call after the one in which the eval
// timeout or Interrupt came.
func newBuiltin(name string, length int, fn func(tifajs.Args) (tifajs.Value, error)) *tifajs.Builtin {
	return intlhook.NewBuiltin(name, length, func(a tifajs.Args) (tifajs.Value, error) {
		intlhook.Check(a.VM())
		if h := callHook; h != nil {
			h(true)
			defer h(false)
		}
		return fn(a)
	})
}
