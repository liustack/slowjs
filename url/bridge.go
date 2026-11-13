// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/whatwgurl"
)

// The Builtins url.js is called with. They keep no state: a URL is javascript's,
// in the VM, under its memory limit, and nothing a script gave is held in Go
// when a call ends. Each reads its strings through the door (door.go) and
// answers undefined when the call is to end: the uncatchable error is then
// pending and the answer is not read.

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

// parseArgs parses argument 0 against argument 1, a base or undefined. ok is
// false when the input is not a URL, a string is longer than MaxUnits, the URL
// would be, or the call is to end.
func parseArgs(a quickjs.Args) (*whatwgurl.URL, bool) {
	in, present, v := admit(a, 2, 0, 0, 1)
	if v != admitted || !present[0] {
		return nil, false
	}
	var base *whatwgurl.URL
	if present[1] {
		b, err := whatwgurl.Parse(in[1], nil)
		if err != nil {
			return nil, false
		}
		base = b
	}
	u, err := whatwgurl.Parse(in[0], base)
	return u, err == nil
}

// parts is the array url.js keeps for a URL: the href, then protocol,
// username, password, host, hostname, port, pathname, search, hash and
// origin, and last the bits of the record the getters do not show, which with
// the parts make the record again (record).
func parts(vm *quickjs.VM, u *whatwgurl.URL) (quickjs.Value, error) {
	p, href := u.Parts()
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
	var vals [len(strs) + 1]quickjs.Value
	for i, s := range strs {
		v, err := emit(vm, s)
		if err != nil {
			for _, f := range vals[:i] {
				f.Free()
			}
			return quickjs.Value{}, err
		}
		vals[i] = v
	}
	vals[len(strs)] = vm.NewInt(flags)
	arr, err := vm.NewArray(vals[:]...)
	for _, v := range vals {
		v.Free()
	}
	return arr, err
}

// parseBuiltin is parse(input, base): the parts of the URL, or undefined.
var parseBuiltin = newBuiltin("parse", 2, func(a quickjs.Args) (quickjs.Value, error) {
	u, ok := parseArgs(a)
	if !ok {
		return quickjs.UndefinedValue, nil
	}
	return parts(a.VM(), u)
})

// canParseBuiltin is canParse(input, base): 1 or 0.
var canParseBuiltin = newBuiltin("canParse", 2, func(a quickjs.Args) (quickjs.Value, error) {
	if _, ok := parseArgs(a); ok {
		return a.VM().NewInt(1), nil
	}
	return a.VM().NewInt(0), nil
})

// errNotParts is the error of set for arguments that url.js does not hand it:
// a part that is not a setter's, or a record that is not a URL's. A script
// cannot reach set (see docs/changes/url.md), so only a fault of url.js could.
var errNotParts = &quickjs.Error{Name: "TypeError", Message: "URL: the record of the URL is not that of a URL"}

// setBuiltin is set(part, value, flags, protocol, username, password,
// hostname, port, pathname, search, hash): the setter of a part on the URL
// whose record are the parts that follow the value, with the bits of parts,
// as the parts of the URL it makes. It answers 0 when the value is longer
// than MaxUnits or the URL would be, and undefined when the call is to end.
// Parts and bits that are not those of a URL, and a part number that is not
// one of the setters, are a TypeError: the URL is not changed.
var setBuiltin = newBuiltin("set", 11, func(a quickjs.Args) (quickjs.Value, error) {
	vm := a.VM()
	pn, ok := a.Number(0)
	part := int(pn)
	if !ok || float64(part) != pn || part < setProtocol || part > setHash {
		return quickjs.Value{}, errNotParts
	}
	fn, ok := a.Number(2)
	f := int(fn)
	if !ok || float64(f) != fn || f&^(hasHost|opaque|hasQuery|hasFragment) != 0 {
		return quickjs.Value{}, errNotParts
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
		return quickjs.Value{}, errNotParts
	case v != admitted:
		return quickjs.UndefinedValue, nil
	}
	for _, ok := range present[:9] {
		if !ok {
			return quickjs.Value{}, errNotParts
		}
	}
	u, err := whatwgurl.FromParts(whatwgurl.Parts{
		Protocol: in[1], Username: in[2], Password: in[3], Hostname: in[4],
		Port: in[5], Pathname: in[6], Search: in[7], Hash: in[8],
		HasHost: f&hasHost != 0, Opaque: f&opaque != 0, HasQuery: f&hasQuery != 0, HasFragment: f&hasFragment != 0,
	})
	if err != nil {
		return quickjs.Value{}, errNotParts
	}
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
	return parts(vm, u)
})

// errTooLong is the error of a name or a value of a query string, or of its
// encoding, that is longer than MaxUnits.
var errTooLong = &quickjs.Error{Name: "RangeError", Message: "URLSearchParams: a name or a value, or its encoding, is longer than 65536 UTF-16 code units"}

// formDecodeBuiltin is formDecode(field): a name or a value of a query string
// as the application/x-www-form-urlencoded parser makes it. url.js splits the
// string at & and = and calls it for a field that has a + or a %.
var formDecodeBuiltin = newBuiltin("formDecode", 1, func(a quickjs.Args) (quickjs.Value, error) {
	in, present, v := admit(a, 0, 0, 0)
	switch {
	case v == tooLong:
		return quickjs.Value{}, errTooLong
	case v != admitted || !present[0]:
		return quickjs.UndefinedValue, nil
	}
	return emit(a.VM(), whatwgurl.FormDecode(in[0]))
})

// formEncodeBuiltin is formEncode(field): a name or a value as the
// application/x-www-form-urlencoded serializer writes it.
var formEncodeBuiltin = newBuiltin("formEncode", 1, func(a quickjs.Args) (quickjs.Value, error) {
	in, present, v := admit(a, 0, 0, 0)
	switch {
	case v == tooLong:
		return quickjs.Value{}, errTooLong
	case v != admitted || !present[0]:
		return quickjs.UndefinedValue, nil
	}
	out, err := whatwgurl.FormEncode(in[0])
	if err != nil {
		return quickjs.Value{}, errTooLong
	}
	return emit(a.VM(), out)
})

// bridge is the list of Builtins url.js is a function of, in its order.
var bridge = []*quickjs.Builtin{parseBuiltin, setBuiltin, formDecodeBuiltin, formEncodeBuiltin, canParseBuiltin}

// callHook, when a test sets it, is called as each Builtin of this package
// starts (enter true) and as it ends.
var callHook func(enter bool)

// newBuiltin is intlhook.NewBuiltin with callHook, and one look at the stop
// flag as each call starts. The engine counts toward its next poll the
// bytecode it runs and not the time a Go function takes, so a loop that calls
// a Builtin taking a few milliseconds would make many such calls between two
// polls: the look ends the script at the call after the one in which the eval
// timeout or Interrupt came.
func newBuiltin(name string, length int, fn func(quickjs.Args) (quickjs.Value, error)) *quickjs.Builtin {
	return intlhook.NewBuiltin(name, length, func(a quickjs.Args) (quickjs.Value, error) {
		intlhook.Check(a.VM())
		if h := callHook; h != nil {
			h(true)
			defer h(false)
		}
		return fn(a)
	})
}
