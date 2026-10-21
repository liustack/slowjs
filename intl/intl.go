// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package intl implements ECMA-402, the ECMAScript Internationalization API,
// for a [quickjs.VM].
//
// QuickJS has no Intl. It is not a disabled build option: neither Bellard's
// engine nor quickjs-ng implements ECMA-402, so a page or bundle that reaches
// for Intl.DateTimeFormat gets a ReferenceError. [Register] installs a Go
// backed implementation of the parts of the API pages depend on:
//
//   - Intl.DateTimeFormat: date and time styles, the component options,
//     hour cycles, day periods, time zone names, and the IANA time zone
//     database behind the timeZone option.
//   - Intl.NumberFormat: grouping, fraction and significant digits, rounding
//     modes, percent, currency, unit, compact and scientific notation, and
//     numbering systems.
//   - Intl.PluralRules: cardinal and ordinal rules, select and selectRange.
//   - Intl.Collator, and String.prototype.localeCompare through it: the
//     collation types, sensitivity, numeric, caseFirst, ignorePunctuation
//     and search usage, from go-intl (internal/gointl), which is ICU's
//     collation in Go over the data Chromium ships.
//   - Intl.Segmenter: graphemes, words, with the dictionaries of Thai, Lao,
//     Khmer, Burmese, Chinese and Japanese, and sentences.
//   - Intl.RelativeTimeFormat: every unit and style, words such as
//     "yesterday" with numeric auto, and numbering systems.
//   - Intl.ListFormat: conjunctions, disjunctions and unit lists in every
//     style, with the joins that depend on the next item, as Spanish "e"
//     and "u" and Hebrew "ו-".
//   - Intl.DisplayNames: the names of languages, with dialect names such
//     as "British English" or without, of regions, scripts, currencies,
//     calendars and date fields, in every style.
//   - Intl.DurationFormat: every style and each unit's own, digital
//     clocks with fractional seconds, and durations given as objects or as
//     Temporal's duration strings ("PT1H30M"), as Chromium takes them.
//   - Intl.getCanonicalLocales and each constructor's supportedLocalesOf.
//
// Register also replaces Date.prototype.toLocaleString, toLocaleDateString and
// toLocaleTimeString along with Number.prototype.toLocaleString,
// BigInt.prototype.toLocaleString and String.prototype.localeCompare, which
// ECMA-402 redefines in terms of those constructors. QuickJS ships fallbacks
// for them that ignore their arguments, so
//
//	new Date(0).toLocaleString("en-US", {timeZone: "America/New_York"})
//
// answers in the host's own zone before Register - a wrong time, silently -
// and "12/31/1969, 7:00:00 PM" after it.
//
// # Locales
//
// The locale data is CLDR 48.2 (cldr-json 48.2.3), for every locale Chromium
// has. A locale resolves as it does in Chromium: one with no data of its own
// falls back to its parent (en-XX to en) or, failing that, to en-US. With no
// locale argument the locale is en-US and with no timeZone option the zone is
// UTC: Intl and the toLocale* methods never read the host's language or zone.
// Neither does the engine's own Date, whose local time is UTC in every VM.
//
// Dates are in the Gregorian calendar, or in the buddhist one for th and the
// persian one for fa, fa-AF, ps, uz-Arab and ckb-IR, whose default they are.
// Another calendar asked for, with -u-ca- or the calendar option, such as
// japanese or roc, is not supported: as ECMA-402 has it for a calendar an
// implementation lacks, it is dropped from the resolved locale, the date is
// in the locale's default calendar, and resolvedOptions().calendar says
// which, so a page can tell. Nothing throws, which would break a page that
// Chromium renders.
//
// Not implemented: Intl.supportedValuesOf and formatRange. NumberFormat and
// PluralRules accept roundingIncrement and roundingPriority but only
// implement their defaults, which is what resolvedOptions reports: a value
// out of range throws a RangeError, any other is ignored.
//
// # Differences from Chromium
//
// The output is compared with Chromium 156 (V8 over ICU 78.2, which has CLDR
// 48.1) on about 60,000 expressions recorded in testdata/chromium. All
// match exactly but those in a calendar other than the ones above, which
// format as Chromium does without that calendar: in the locale's default
// calendar, Gregorian but for th and the persian locales.
//
// Checks over every locale beyond those expressions found these other
// differences, all in rare locales or options:
//
//   - sr-Cyrl-ME names currencies, units and time zones in Cyrillic, where
//     Chromium writes them in Latin ("1 литар" for "1 litar").
//   - uz-AF and uz-Arab-AF put an era, "AP", before the year of a date with
//     time, which Chromium leaves out, and do not pad the hour of a time
//     with a long zone name ("۵:۳۰" for "۰۵:۳۰").
//   - en-IE and fr-MA with hourCycle h11 write the day period "am" and "AM"
//     where Chromium writes "a.m.".
//   - om-KE's short era is "BA" where Chromium's is "CE".
//
// Intl.Segmenter breaks words by ICU's rules, which Chromium changes: there a
// full stop or a colon between letters ends a word, so "U.S.A" is five
// segments and "a:b" three, and Lao and Khmer break by smaller dictionaries
// of Chromium's own, which find other words in some runs.
//
// Intl.DisplayNames names more than Chromium, whose ICU data leaves names
// out: of some languages ("tlh" for Klingon), of variants ("German (1996)"),
// of numeric regions but 419, and of currencies no longer in use, which it
// writes as their codes, or leaves undefined with fallback none. It has no
// names at all for ak, az, bs, kk, kok, pa, sd and uz, which it names as
// root does, by codes, or in English. And it has names of its own for Hong
// Kong, Macao, Palestine and the Falkland Islands ("Hong Kong" for "Hong
// Kong SAR China"). Here the names are CLDR's. The narrow names of de-CH's
// era and th's time zone are go-intl's, which ICU writes otherwise.
//
// Intl.DurationFormat writes a fraction of more than 2**63 nanoseconds, as
// in {nanoseconds: 1e20} with numeric seconds, as V8 does on x86-64:
// "9223372036.854775808" seconds. V8 on arm64 writes
// "9223372036.854775807".
//
// Error messages are worded differently, though the errors thrown are the
// same.
//
// Where Chromium's ICU data departs from CLDR 48.2, the data here follows
// Chromium. A language Chromium ships no dates or numbers for formats them
// with CLDR root's data, as az and kk do ("2026 M01 5, Mon"), and one it ships
// no currency, zone, unit or region names for takes them from English. Digits
// a locale does not use by default, such as arab in de, take the symbols and
// patterns Chromium has for them, which cldr-json leaves out. A
// zone's city is the one its id spells ("Sao Paulo"), Hong Kong
// and Macao go by their short names, and the zones of the British Isles are
// never "Greenwich Mean Time". fr-CH groups digits with a narrow no-break
// space (CLDR 48.1's), ko calls the North American zones 북미 (North
// America), and uk and ru write the hryvnia as грн and грн. As in Chromium,
// narrow and thin spaces in dates are written as plain spaces.
//
// # Cost
//
// The locale data is embedded in the binary, about 1.3 MB, and stays in its
// read only data. A language is decoded into the Go heap the first time a
// formatter needs it, once per process, and every VM shares it: 100 to 180
// KB per language, and about 1 MB for the index and the data all languages
// share on the first use of any. Register itself only puts stand-ins on the
// VM. The javascript half is installed the first time a page touches Intl or
// a toLocale* method, from bytecode compiled once per process, so a VM that
// never formats anything does not carry it. A formatter is resolved once per
// process for the same locales and options, whichever VM asks, and kept for
// all of them: the process keeps up to 1024 of them, dropping the oldest, and
// an object whose formatter was dropped resolves it again on its next call.
// The dropping is the process's, not a VM's: a page making formatters with
// options made up in a loop makes the formatters of other VMs resolve again
// too, which costs them time but changes no result.
//
// # License of the data
//
// The embedded locale data comes from Unicode CLDR and from the ICU data
// Chromium ships, which are under the Unicode License V3, with changes
// Chromium made to it under Chromium's BSD license. A program that imports
// this package distributes that data, and has to include the notices in
// LICENSE-DATA, in this package's directory, in its documentation or with
// its copies.
//
// # Time zones
//
// Resolving an IANA name needs the zoneinfo database, which Go reads from the
// operating system or, when the program imports time/tzdata, from the copy
// embedded in the binary:
//
//	import _ "time/tzdata"
//
// A binary that has to run on Windows or in a scratch container wants that
// import - and it belongs in that binary's main package, not in a library.
// Importing it from a library adds a few hundred KB to every program that
// links the library and takes the choice away from the ones that would rather
// use the host's database, so this package does not import it.
//
// Offsets come from that database. Whether a time is daylight saving time,
// which picks a zone's daylight or standard name, is what the rearguard form
// of the tz data says, as in ICU: Linux and time/tzdata ship the main form,
// where Europe/Dublin's winter time is daylight saving time, and macOS the
// rearguard one. For the few zones whose two forms differ, the answer comes
// from a table of tzdb 2026d, the release in Chromium 156's ICU.
//
// The default zone, which is also what
// Intl.DateTimeFormat().resolvedOptions().timeZone reports, is UTC unless
// [TimeZone] sets another. Intl never consults the host's zone (see Locales
// for the engine's Date, which does).
package intl // import "github.com/liustack/quickjs/intl"

import (
	_ "embed"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/liustack/quickjs"
)

//go:embed intl.js
var shim string

//go:embed stub.js
var stub string

// script is javascript compiled to bytecode once per process and evaluated
// from there on every VM: the bytecode does not depend on the VM that made it.
type script struct {
	src *string

	mu   sync.Mutex
	code []byte // nil until a compile works
}

func (s *script) eval(vm *quickjs.VM) error {
	code, err := s.bytecode()
	if err != nil {
		return err
	}

	_, err = vm.EvalBytecode(code)
	return err
}

// bytecode returns s compiled. A compile that fails is not kept, the next
// call tries again.
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

// compile compiles src on a VM of its own. The VM that needs the bytecode may
// be short of memory, stack or time, and a compile there would fail for
// reasons that have nothing to do with src. The functions are compiled
// without their source and debug information: they stand for built-ins, whose
// toString shows [native code], and each VM that loads them carries less.
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

	// interruptedInstalls counts the installs that started over after an
	// interrupt or running out of memory cut one short, for the tests.
	interruptedInstalls atomic.Int64
)

type config struct {
	timeZone string
}

// An Option configures [Register].
type Option func(*config)

// TimeZone returns an Option setting the default time zone: the one a
// formatter created without a timeZone option uses and the one
// Intl.DateTimeFormat().resolvedOptions().timeZone reports. The name must be
// an IANA identifier like "Europe/Prague", "UTC", or an offset like "+05:30".
//
// Without it the zone is UTC. Intl never reads the host's zone: a server would
// otherwise render a page differently depending on where it runs. The
// engine's Date local time methods do not follow this option: their local
// time is always UTC.
func TimeZone(name string) Option { return func(c *config) { c.timeZone = name } }

// Register installs Intl on vm's global object and replaces the toLocale*
// methods ECMA-402 defines in terms of it. See the package documentation for
// what is and is not implemented.
//
// Call it on every VM that is to have Intl, including one created to replace
// another. A VM is a javascript realm, Intl lives on that realm's global
// object and it does not migrate: an embedder that gives each document a fresh
// realm - the only way to empty one, see [quickjs.VM] - has to register per
// document rather than once at startup, or the second document is back to a
// ReferenceError.
//
// Call it before any code of the page runs on vm. It takes then the
// built-ins Intl's code uses, so that a page changing Array.prototype,
// Object.prototype, JSON and the like afterwards changes nothing Intl does:
// called later, it takes what the page changed them to.
//
// Registering a second time on the same VM is an error rather than a silent
// no-op, because it would otherwise drop that call's options. Nothing has to
// be released: vm.Close does it all. The formatters themselves belong to the
// process, which shares them between VMs and keeps a bounded number of them.
func Register(vm *quickjs.VM, opts ...Option) error {
	switch already, err := registered(vm); {
	case err != nil:
		return err
	case already:
		return fmt.Errorf("intl: this VM already has Intl; a VM created to replace a realm needs its own Register")
	}

	c := &config{}
	for _, o := range opts {
		o(c)
	}
	if c.timeZone == "" {
		c.timeZone = "UTC"
	}
	name, _, err := timeZone(c.timeZone)
	if err != nil {
		return err
	}

	// stub.js is a function of the install Builtin and the zone, which puts
	// the stand-ins in place. The shim is installed the first time the page
	// touches Intl or a toLocale* method, from inside that call.
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

	_, err = f.Call(quickjs.UndefinedValue, install, name)
	return err
}

// installing holds the VMs an install is running on.
var installing sync.Map

// installBuiltin installs the shim: it evaluates intl.js, a function of the
// built-ins stub.js took, the zone, the option lists and the bridge, and
// returns what calling it returns, the objects stub.js then puts in place.
// Its third argument says the previous install was cut short.
var installBuiltin = quickjs.NewBuiltin("install", 3, func(a quickjs.Args) (quickjs.Value, error) {
	vm := a.VM()
	if _, busy := installing.LoadOrStore(vm, true); busy {
		// The shim calls no code of the page while it installs, so nothing
		// can get here.
		return quickjs.Value{}, &quickjs.Error{Name: "Error", Message: "intl: used while being installed"}
	}

	defer installing.Delete(vm)

	if retry, _ := a.Bool(2); retry {
		interruptedInstalls.Add(1)
	}
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

	args := make([]any, 0, 3+len(bridge))
	args = append(args, a.Value(0), a.Value(1), optionLists)
	for _, b := range bridge {
		v, err := vm.NewBuiltinFunction(b)
		if err != nil {
			return quickjs.Value{}, err
		}

		defer v.Free()

		args = append(args, v)
	}
	return f.CallValue(quickjs.UndefinedValue, args...)
})

// marker is the well known symbol stub.js stamps on the global object of a VM
// it registers, keyed by this package's import path.
const marker = "modernc.org/quickjs/intl"

// registered reports whether this package has already installed Intl on vm.
func registered(vm *quickjs.VM) (bool, error) {
	v, err := vm.Eval("globalThis[Symbol.for("+strconv.Quote(marker)+")] === true", quickjs.EvalGlobal)
	if err != nil {
		return false, err
	}

	b, _ := v.(bool)
	return b, nil
}
