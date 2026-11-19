// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package intl gives a [slowjs.VM] the ECMA-402 Intl API, computed in Go.
//
// QuickJS comes without Intl. [Register] adds it: the constructors
// Intl.DateTimeFormat, NumberFormat, PluralRules, Collator, Segmenter,
// RelativeTimeFormat, ListFormat, DisplayNames, DurationFormat and Locale,
// the supportedLocalesOf of each but Locale, Intl.getCanonicalLocales and
// Intl.supportedValuesOf. It also replaces the methods that format with a
// locale: Date.prototype.toLocaleString, toLocaleDateString and
// toLocaleTimeString, Number.prototype.toLocaleString,
// BigInt.prototype.toLocaleString, String.prototype.localeCompare, and
// String.prototype.toLocaleUpperCase and toLocaleLowerCase, which follow the
// case rules of Turkish, Azerbaijani, Lithuanian and Greek.
//
// # Locales and time zones
//
// Locales resolve as in Chromium: a locale without data of its own falls
// back to its parent, and then to en-US. With no locale given the locale is
// en-US, and with no time zone given the zone is the VM's default, UTC
// unless [TimeZone] sets another. Nothing is read from the host: not its
// locale, not its zone, not the TZ variable. The zone data is tz 2026e,
// built into go-intl, so the program needs neither the host's zoneinfo nor
// time/tzdata.
//
// # Compared with Chromium
//
// The answers of DateTimeFormat, NumberFormat and PluralRules are
// Chromium 156's, checked against what that browser answered for tens of
// thousands of expressions (testdata/chromium). The known differences are
// elsewhere: where Segmenter breaks words, a few names of DisplayNames, the
// 001 time zones, -u-va-posix and unknown -u-rg- regions of Locale, the
// currencies SLE, XCG and ZWG that supportedValuesOf("currency") lists in
// addition, how DurationFormat writes a sum of nanoseconds past 2**63, and
// the words of error messages. Under tz 2026e, three answers for
// America/Winnipeg differ from that browser's older data.
//
// # Cost
//
// The data takes six packs, 9.35 MB of the binary in all. The build tags
// slowjs_intl_nonames and slowjs_intl_nocjdict leave out the display names
// and the Chinese and Japanese word dictionary of Segmenter.
//
// The formatters are shared by all the VMs of the process: a registry keeps
// up to 1024 of them, and an object whose formatter it dropped makes it
// again, from the same arguments, on its next call, with the same results.
// A date formatter is told apart by the pattern it writes with, beside its
// options. docs/changes/time-and-intl.md has the timings.
//
// # License of the data
//
// A program that imports this package carries data from Unicode CLDR, ICU
// and Chromium, and has to ship intl/LICENSE-DATA and the notices of
// internal/gointl with it. NOTICE.md says which covers what.
package intl // import "github.com/liustack/slowjs/intl"

import (
	_ "embed"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/internal/intlhook"
)

// intl.js is the javascript half of Intl, installed on a VM when a page
// first uses Intl or a method it replaces. stub.js puts the stand-ins in
// place that install it.

//go:embed intl.js
var shim string

//go:embed stub.js
var stub string

// A script is javascript compiled once per process, to bytecode every VM
// evaluates.
type script struct {
	src *string

	mu   sync.Mutex
	code []byte
}

var (
	shimScript = script{src: &shim}
	stubScript = script{src: &stub}

	// shimInstalls counts the installs of intl.js begun, and
	// interruptedInstalls those begun after one on the same VM was cut short,
	// for the tests.
	shimInstalls, interruptedInstalls atomic.Int64
)

// bytecode returns the compiled script. A compile that fails is not kept,
// and the next call tries again.
func (s *script) bytecode() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.code != nil {
		return s.code, nil
	}

	// A VM of its own: the one that wants the script may be short of
	// memory, stack or time.
	vm, err := slowjs.NewVM()
	if err != nil {
		return nil, err
	}

	defer vm.Close()

	// Without source and debug information, its functions show
	// [native code], as built-ins do, and each VM holds less.
	vm.SetStrip(slowjs.StripDebug)
	code, err := vm.Compile(*s.src, slowjs.EvalGlobal)
	if err != nil {
		return nil, err
	}

	s.code = code
	return code, nil
}

// eval evaluates the script on vm.
func (s *script) eval(vm *slowjs.VM) error {
	code, err := s.bytecode()
	if err != nil {
		return err
	}

	_, err = vm.EvalBytecode(code)
	return err
}

type config struct {
	zone string
}

// An Option configures [Register].
type Option func(*config)

// TimeZone sets the default time zone of the VM: the one a DateTimeFormat
// without a timeZone option formats in, and the one its resolvedOptions
// reports. name is an IANA name, such as "Europe/Prague", "UTC", or an
// offset, such as "+05:30". Without it the default is UTC, whatever the
// host's zone is.
//
// The local time of the engine's Date is UTC whatever the zone: getHours,
// toString and the other methods of Date that are not toLocale* do not
// follow it.
func TimeZone(name string) Option {
	return func(c *config) { c.zone = name }
}

// marker is the key, as Symbol.for(marker), of the property stub.js puts
// on the global object of a VM it registered.
const marker = "github.com/liustack/slowjs/intl"

// registered reports whether vm has Intl from Register.
func registered(vm *slowjs.VM) (bool, error) {
	v, err := vm.Eval("globalThis[Symbol.for("+strconv.Quote(marker)+")] === true", slowjs.EvalGlobal)
	if err != nil {
		return false, err
	}

	b, _ := v.(bool)
	return b, nil
}

// Register gives vm Intl and the locale-aware methods of Date, Number,
// BigInt and String (see the package documentation).
//
// Every VM needs its own Register, a VM made to replace the realm of
// another one included. Call it before any code of the page runs: it takes
// the built-ins Intl works with then, so that a page that later changes
// Array.prototype, JSON or the like does not change Intl. A second Register
// on the same VM returns an error.
//
// Register only puts stand-ins in place. Intl is installed the first time
// a page uses it or one of the methods. Nothing needs to be released:
// vm.Close frees it all.
func Register(vm *slowjs.VM, opts ...Option) error {
	switch done, err := registered(vm); {
	case err != nil:
		return err
	case done:
		return errors.New("intl: this VM already has Intl; a VM that replaces a realm needs a Register of its own")
	}

	c := config{}
	for _, o := range opts {
		o(&c)
	}
	if c.zone == "" {
		c.zone = "UTC"
	}
	zone, err := resolveTimeZone(c.zone)
	if err != nil {
		return err
	}

	code, err := stubScript.bytecode()
	if err != nil {
		return err
	}

	f, err := vm.EvalBytecodeValue(code)
	if err != nil {
		return err
	}

	defer f.Free()

	install, err := vm.NewBuiltinFunction(installBuiltin)
	if err != nil {
		return err
	}

	defer install.Free()

	state, err := intlhook.State(vm)
	if err != nil {
		return err
	}

	defer state.Free()

	if _, err := f.Call(slowjs.UndefinedValue, install, zone, state); err != nil {
		return err
	}

	rememberZone(vm, zone)
	return nil
}

// A request is what a constructor resolves from: the locales the page asked
// for, and the options intl.js read and converted, only those it was given,
// each a string, a float64 or a bool. Its json is how the registry tells
// formatters apart before they are made.
type request struct {
	Locales []string       `json:"locales"`
	Options map[string]any `json:"options"`

	// vm is the VM of the call, nil in some tests.
	vm *slowjs.VM
}

// installing holds the VMs intl.js is being installed on.
var installing sync.Map // *slowjs.VM -> bool

// installBuiltin installs intl.js on the VM of its call, for stub.js, which
// passes the built-ins it took, the default zone, whether an install of
// this VM was cut short before, and the state the VM shares with Temporal.
// It returns what intl.js returns, which stub.js puts in place of the
// stand-ins.
var installBuiltin = intlhook.NewBuiltin("install", 3, func(a slowjs.Args) (slowjs.Value, error) {
	vm := a.VM()
	if _, busy := installing.LoadOrStore(vm, true); busy {
		// intl.js calls no code of the page while it installs, so only a
		// bug gets here.
		return slowjs.Value{}, &slowjs.Error{Name: "Error", Message: "intl: Intl was used while it was being installed"}
	}

	defer installing.Delete(vm)

	if retry, _ := a.Bool(2); retry {
		interruptedInstalls.Add(1)
	}
	shimInstalls.Add(1)

	code, err := shimScript.bytecode()
	if err != nil {
		return slowjs.Value{}, err
	}

	g, err := vm.EvalBytecodeValue(code)
	if err != nil {
		return slowjs.Value{}, err
	}

	defer g.Free()

	args := make([]any, 0, 4+len(bridge))
	args = append(args, a.Value(0), a.Value(1), optionLists, a.Value(3))
	for _, b := range bridge {
		v, err := vm.NewBuiltinFunction(b)
		if err != nil {
			freeValues(args[4:])
			return slowjs.Value{}, err
		}
		args = append(args, v)
	}

	defer freeValues(args[4:])

	return g.CallValue(slowjs.UndefinedValue, args...)
})

// freeValues frees the Values in l.
func freeValues(l []any) {
	for _, v := range l {
		if v, ok := v.(slowjs.Value); ok {
			v.Free()
		}
	}
}
