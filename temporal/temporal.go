// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package temporal implements Temporal, the ECMAScript date and time API,
// for a [quickjs.VM].
//
// QuickJS has no Temporal. [Register] installs all of it: Temporal.Now,
// Instant, ZonedDateTime, PlainDate, PlainTime, PlainDateTime,
// PlainYearMonth, PlainMonthDay and Duration with every method V8 ships, the
// calendars of ECMA-402 and the IANA time zones, and
// Date.prototype.toTemporalInstant. The computing is go-intl's temporal
// package (internal/gointl/temporal), a Go port of temporal_rs, the library
// V8 builds Temporal on. The javascript half reads arguments, options and
// property bags in the order V8 reads them, with its conversions and its
// error messages, and keeps the internal slots of each object in a private
// field no page can reach.
//
// # Time and zones
//
// Temporal.Now reads the clock [Clock] sets, the host's by default. Only
// Temporal.Now reads it: Date.now, new Date() and Intl keep the host's
// clock.
//
// Temporal.Now.timeZoneId() is the VM's default time zone, the one package
// intl formats in (intl.TimeZone, "UTC" without it), which is ECMA-402's
// DefaultTimeZone, and the Now methods called without a zone use it too.
// The engine's Date does not follow it: its local time is UTC in every VM,
// so new Date().getHours() and Temporal.Now.plainTimeISO().hour differ
// when the default zone is not UTC.
//
// The zone and calendar data is go-intl's, built into the binary: ICU's
// zoneinfo64 from ICU's time zone update for tz 2026e and ICU's calendar
// data, the 92 KB pack of it that Temporal reads. The host's zoneinfo is never read, so Temporal gives the
// same answers on every machine, a container without /usr/share/zoneinfo
// included. Temporal date formatting and named-zone validation read the
// same embedded zones, without needing host zoneinfo.
// ZonedDateTime.toLocaleString honors Temporal's offset even if the zone
// data used for formatting ever differs, naming that offset instead.
//
// # toLocaleString
//
// Date types use the same type-specific formats as Intl.DateTimeFormat,
// from package intl, whether or not the VM has intl registered. Plain values
// ignore timeZone, an Instant uses the formatter's zone, and ZonedDateTime
// uses its own zone. DateTimeFormat itself rejects ZonedDateTime.
// Formatting supports all 18 calendars listed by Chromium. A mismatched
// calendar throws RangeError.
// ISO PlainYearMonth and PlainMonthDay need an explicit ISO calendar.
// Duration.toLocaleString uses Intl.DurationFormat. Both paths read private
// slots and captured built-ins, not properties a page can replace.
//
// # Cost
//
// Register puts a stand-in for the global Temporal and for
// Date.prototype.toTemporalInstant on the VM. The rest is installed the
// first time a page reads Temporal or calls toTemporalInstant, from
// bytecode compiled once per process. The zone and calendar data is read
// once per process, the first time any VM needs it.
//
// # License of the data
//
// The zone and calendar data is go-intl's, made from ICU's data and the IANA
// tz database, and the computing is go-intl's port of temporal_rs, of
// ICU4X's crates and of parts of V8. They are under the Unicode License V3
// and the licenses of those projects. A program that imports this package
// distributes them, and has to include in its documentation or with its
// copies go-intl's LICENSE (internal/gointl/LICENSE) and the notices in
// internal/gointl/qjsdata: NOTICE-ICU, NOTICE-ICU4X, NOTICE-TEMPORAL and
// NOTICE-V8. It imports package intl, so intl's LICENSE-DATA as well.
// NOTICE.md, at the root of the module, says which notice covers what.
package temporal // import "github.com/liustack/quickjs/temporal"

import (
	_ "embed"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	_ "github.com/liustack/quickjs/intl" // toLocaleString
)

//go:embed temporal.js
var shim string

//go:embed stub.js
var stub string

// script is javascript compiled to bytecode once per process and evaluated
// from there on every VM, as package intl does.
type script struct {
	src *string

	mu   sync.Mutex
	code []byte
}

func (s *script) bytecode() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.code == nil {
		code, err := compile(*s.src)
		if err != nil {
			return nil, err
		}
		s.code = code
	}
	return s.code, nil
}

// compile compiles src on a VM of its own, without its source and debug
// information: the functions stand for built-ins, whose toString shows
// [native code].
func compile(src string) ([]byte, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}

	defer vm.Close()

	vm.SetStrip(quickjs.StripDebug)
	return vm.Compile(src, quickjs.EvalGlobal)
}

var (
	shimScript = script{src: &shim}
	stubScript = script{src: &stub}

	// shimInstalls counts the VMs the shim has been installed on, for the
	// tests.
	shimInstalls atomic.Int64
)

type config struct {
	clock func() time.Time
}

// An Option configures [Register].
type Option func(*config)

// Clock returns an Option setting the clock Temporal.Now reads, which is
// time.Now without it. A server rendering a page at a fixed time, or a test,
// sets one. now is called on the goroutine running the VM, once per
// Temporal.Now call, and must be safe for concurrent use when VMs share it.
// A time outside Temporal.Instant's range, 1e8 days either side of 1970,
// makes Temporal.Now throw a RangeError.
//
// now must not block: the VM waits for it, and an eval timeout
// (SetEvalTimeout) cannot cut it short, since the engine checks for one
// only between its own instructions. A panic in now reaches the caller of
// Eval, as a panic in any Go function the VM calls does, and the VM can be
// used again after it.
//
// Clock drives Temporal.Now only: Date.now, new Date() and Intl keep the
// host's clock.
func Clock(now func() time.Time) Option { return func(c *config) { c.clock = now } }

// clocks are the clocks of the VMs registered with one, until they are
// closed. Close drops the entry (VM.OnClose): a clock may reference its VM,
// so the entry would keep the VM reachable if it were left to a cleanup of
// the garbage collector.
var clocks sync.Map // *quickjs.VM -> func() time.Time

func clockOf(vm *quickjs.VM) func() time.Time {
	if c, ok := clocks.Load(vm); ok {
		return c.(func() time.Time)
	}
	return time.Now
}

// Register installs Temporal on vm's global object, and
// Date.prototype.toTemporalInstant. See the package documentation for what
// it is made of.
//
// As with intl.Register: call it on every VM that is to have Temporal,
// before any code of the page runs, so that it takes the built-ins its code
// uses before a page can change them. Registering a second time on the
// same VM is an error. Nothing has to be released: vm.Close does it all.
func Register(vm *quickjs.VM, opts ...Option) error {
	switch already, err := registered(vm); {
	case err != nil:
		return err
	case already:
		return errors.New("temporal: this VM already has Temporal; a VM created to replace a realm needs its own Register")
	}

	c := &config{}
	for _, o := range opts {
		o(c)
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

	if _, err := f.Call(quickjs.UndefinedValue, install, state); err != nil {
		return err
	}

	if c.clock != nil {
		clocks.Store(vm, c.clock)
		vm.OnClose(func() { clocks.Delete(vm) })
	}
	return nil
}

// installing holds the VMs an install is running on.
var installing sync.Map

// installBuiltin installs the shim: it evaluates temporal.js, a function of
// the built-ins stub.js took, the names of the operations, the bridge and
// what toLocaleString needs of package intl, and returns what calling it
// returns, the objects stub.js then puts in place.
var installBuiltin = quickjs.NewBuiltin("install", 2, func(a quickjs.Args) (quickjs.Value, error) {
	vm := a.VM()
	if _, busy := installing.LoadOrStore(vm, true); busy {
		return quickjs.Value{}, &quickjs.Error{Name: "Error", Message: "temporal: used while being installed"}
	}

	defer installing.Delete(vm)

	shimInstalls.Add(1)
	code, err := shimScript.bytecode()
	if err != nil {
		return quickjs.Value{}, err
	}

	f, err := vm.EvalBytecodeValue(code)
	if err != nil {
		return quickjs.Value{}, err
	}

	defer f.Free()

	b, err := vm.NewBuiltinFunction(bridge)
	if err != nil {
		return quickjs.Value{}, err
	}

	defer b.Free()

	format, err := vm.NewBuiltinFunction(intlhook.Format)
	if err != nil {
		return quickjs.Value{}, err
	}

	defer format.Free()

	resolveDuration, err := vm.NewBuiltinFunction(intlhook.DurationResolve)
	if err != nil {
		return quickjs.Value{}, err
	}
	defer resolveDuration.Free()
	formatDuration, err := vm.NewBuiltinFunction(intlhook.DurationFormat)
	if err != nil {
		return quickjs.Value{}, err
	}
	defer formatDuration.Free()
	return f.CallValue(quickjs.UndefinedValue, a.Value(0), opNames, b, intlhook.Options, format, intlhook.DefaultZone(vm),
		intlhook.DurationOptions, resolveDuration, formatDuration, a.Value(1))
})

// marker is the well known symbol stub.js stamps on the global object of a
// VM it registers.
const marker = "modernc.org/quickjs/temporal"

func registered(vm *quickjs.VM) (bool, error) {
	v, err := vm.Eval("globalThis[Symbol.for("+strconv.Quote(marker)+")] === true", quickjs.EvalGlobal)
	if err != nil {
		return false, err
	}

	b, _ := v.(bool)
	return b, nil
}
