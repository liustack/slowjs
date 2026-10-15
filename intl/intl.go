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
//   - Intl.getCanonicalLocales and each constructor's supportedLocalesOf.
//
// Register also replaces Date.prototype.toLocaleString, toLocaleDateString and
// toLocaleTimeString along with Number.prototype.toLocaleString and
// BigInt.prototype.toLocaleString, which ECMA-402 redefines in terms of those
// constructors. QuickJS ships fallbacks for them that ignore their arguments,
// so
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
// UTC: the host's language and zone are never read, so a page renders the
// same on every server.
//
// Dates are in the Gregorian calendar, or in the buddhist one for th and the
// persian one for fa, fa-AF, ps, uz-Arab and ckb-IR, whose default they are.
// Any other calendar asked for, with -u-ca- or the calendar option, is
// dropped from the resolved locale and the date is Gregorian.
//
// Not implemented: Intl.Collator, Intl.Segmenter, Intl.RelativeTimeFormat,
// Intl.ListFormat, Intl.DisplayNames, Intl.DurationFormat,
// Intl.supportedValuesOf and formatRange. String.prototype.localeCompare keeps
// QuickJS's code point ordering. NumberFormat accepts roundingIncrement and
// roundingPriority but only implements their defaults, which is what
// resolvedOptions reports.
//
// # Differences from Chromium
//
// The output is compared with Chromium 156 (V8 over ICU 78.2, which has CLDR
// 48.1) on about 52,000 expressions recorded in testdata/chromium. All but
// these match exactly:
//
//   - fr-CH groups digits with an apostrophe, CLDR 48.2's separator, where
//     Chromium has a narrow no-break space.
//   - A numbering system a locale has no symbols for, such as arab in de,
//     takes the locale's latn symbols. ICU takes CLDR root's, which
//     cldr-json does not publish.
//   - A calendar other than the ones above formats as Gregorian.
//   - ko calls the North American zones 미 (America) where Chromium has
//     북미 (North America).
//
// Where Chromium's ICU data departs from CLDR, the data here follows
// Chromium: a zone's city is the one its id spells ("Sao Paulo"), Hong Kong
// and Macao go by their short names, and the zones of the British Isles are
// never "Greenwich Mean Time". As in Chromium, narrow and thin spaces in dates
// are written as plain spaces.
//
// # Cost
//
// The locale data is embedded in the binary, about 1.6 MB, and stays in its
// read only data. A language is decoded into the Go heap the first time a
// formatter needs it, once per process, and every VM shares it: 100 to 180
// KB per language, and about 1 MB for the index and the data all languages
// share on the first use of any. Register itself only puts stand-ins on the
// VM. The javascript half is installed the first time a page touches Intl or
// a toLocale* method, from bytecode compiled once per process, so a VM that
// never formats anything does not carry it.
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
// use the host's database, so this package does not import it. The default
// zone, which is also what
// Intl.DateTimeFormat().resolvedOptions().timeZone reports, is UTC unless
// [TimeZone] sets another. The host's zone is never consulted, so a page
// renders the same on every machine.
package intl // import "github.com/liustack/quickjs/intl"

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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
// reasons that have nothing to do with src.
func compile(src string) ([]byte, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}

	defer vm.Close()

	return vm.Compile(src, quickjs.EvalGlobal)
}

var (
	shimScript = script{src: &shim}
	stubScript = script{src: &stub}

	// shimInstalls counts the VMs the shim has been installed on, for the
	// tests.
	shimInstalls atomic.Int64
)

// hostFuncs are the names the shim reads its host functions from. stub.js
// takes them off the global object as soon as Register has put them there,
// and puts them back only for the moment the shim takes them for good. Both
// scripts list the same names.
var hostFuncs = []string{
	"__quickjs_intl_dtfResolve__",
	"__quickjs_intl_dtfFormat__",
	"__quickjs_intl_nfResolve__",
	"__quickjs_intl_nfFormat__",
	"__quickjs_intl_canonicalLocales__",
	"__quickjs_intl_prResolve__",
	"__quickjs_intl_prSelect__",
}

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
// Without it the zone is UTC. The host's zone is never read: a server would
// otherwise render a page differently depending on where it runs.
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
// Registering a second time on the same VM is an error rather than a silent
// no-op, because it would otherwise drop that call's options. The formatters
// hold no state outside the VM, so nothing has to be released; vm.Close does
// it all.
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

	c.timeZone = name
	for i, fn := range []quickjs.HostFunc{
		c.dtfResolve,
		c.dtfFormat,
		c.nfResolve,
		c.nfFormat,
		c.canonicalLocales,
		c.prResolve,
		c.prSelect,
	} {
		if err := vm.RegisterHostFunc(hostFuncs[i], fn); err != nil {
			return err
		}
	}
	// The shim is installed the first time the page touches Intl or a
	// toLocale* method, from inside that call. Until then the VM carries only
	// the stand-ins stub.js defines.
	install := func([]any) (any, error) {
		shimInstalls.Add(1)
		return nil, shimScript.eval(vm)
	}
	if err := vm.RegisterHostFunc("__quickjs_intl_install__", install); err != nil {
		return err
	}

	return stubScript.eval(vm)
}

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

// dtfResolve resolves the options of a new Intl.DateTimeFormat and returns the
// state format calls will be made with.
func (c *config) dtfResolve(args []any) (any, error) {
	var req request
	if err := unmarshalArg(args, 0, &req); err != nil {
		return failure(err)
	}

	f, err := resolveDateTimeFormat(req, c.timeZone)
	if err != nil {
		return failure(err)
	}

	return resolved(f)
}

// dtfFormat formats a time value with the state dtfResolve produced.
func (c *config) dtfFormat(args []any) (any, error) {
	f, err := cached(args, &dateTimeFormats)
	if err != nil {
		return failure(err)
	}

	ms, ok := timeArg(arg(args, 1))
	if !ok {
		return failure(rangeError("invalid time value"))
	}

	parts, err := f.format(ms)
	if err != nil {
		return failure(err)
	}

	return formatted(parts, arg(args, 2) == true)
}

// nfResolve resolves the options of a new Intl.NumberFormat and returns the
// state format calls will be made with.
func (c *config) nfResolve(args []any) (any, error) {
	var req request
	if err := unmarshalArg(args, 0, &req); err != nil {
		return failure(err)
	}

	f, err := resolveNumberFormat(req)
	if err != nil {
		return failure(err)
	}

	return resolved(f)
}

// nfFormat formats a number with the state nfResolve produced.
func (c *config) nfFormat(args []any) (any, error) {
	f, err := cached(args, &numberFormats)
	if err != nil {
		return failure(err)
	}

	parts, err := f.format(arg(args, 1))
	if err != nil {
		return failure(err)
	}

	return formatted(parts, arg(args, 2) == true)
}

// prResolve resolves the options of a new Intl.PluralRules.
func (c *config) prResolve(args []any) (any, error) {
	var req request
	if err := unmarshalArg(args, 0, &req); err != nil {
		return failure(err)
	}

	p, err := resolvePluralRules(req)
	if err != nil {
		return failure(err)
	}

	return resolved(p)
}

// prSelect implements select, given one number, and selectRange, given two.
func (c *config) prSelect(args []any) (any, error) {
	p, err := cached(args, &pluralFormats)
	if err != nil {
		return failure(err)
	}

	if len(args) > 2 {
		return p.selectRange(args[1], args[2]), nil
	}

	return p.selectNumber(arg(args, 1)), nil
}

// canonicalLocales implements Intl.getCanonicalLocales.
func (c *config) canonicalLocales(args []any) (any, error) {
	var tags []string
	if err := unmarshalArg(args, 0, &tags); err != nil {
		return failure(err)
	}

	if svc, ok := arg(args, 1).(string); ok {
		// supportedLocalesOf, for the constructor named by svc.
		r, err := supportedLocales(tags, map[string]service{
			"NumberFormat":   numberService,
			"DateTimeFormat": dateService,
			"PluralRules":    pluralService,
		}[svc])
		if err != nil {
			return failure(err)
		}

		return marshal(r)
	}

	r := []string{}
	for _, tag := range tags {
		s, err := canonicalLocale(tag)
		if err != nil {
			return failure(err)
		}

		if !slices.Contains(r, s) {
			r = append(r, s)
		}
	}
	return marshal(r)
}

// formatCache memoizes the formatters parsed back from the json state the shim
// keeps, which would otherwise be unmarshaled again on every single format
// call - the largest cost of one. A resolved formatter is immutable, so the
// cache is shared by every VM in the process.
type formatCache[T any] struct {
	mu sync.Mutex
	m  map[string]*T
}

// formatCacheLimit bounds the two caches. Nothing evicts by age: the entries
// are small and a program cycling through more than this many distinct
// formatters is not one the cache can help anyway.
const formatCacheLimit = 256

var (
	dateTimeFormats formatCache[dateTimeFormat]
	numberFormats   formatCache[numberFormat]
	pluralFormats   formatCache[pluralFormat]
)

// cached returns the formatter the json in args[0] describes.
func cached[T any](args []any, c *formatCache[T]) (*T, error) {
	s, ok := arg(args, 0).(string)
	if !ok {
		return nil, errors.New("internal error: expected a json argument")
	}

	c.mu.Lock()

	defer c.mu.Unlock()

	if f, ok := c.m[s]; ok {
		return f, nil
	}

	f := new(T)
	if err := json.Unmarshal([]byte(s), f); err != nil {
		return nil, err
	}

	if i, ok := any(f).(interface{ init() error }); ok {
		if err := i.init(); err != nil {
			return nil, err
		}
	}

	if c.m == nil || len(c.m) >= formatCacheLimit {
		c.m = map[string]*T{}
	}
	c.m[s] = f
	return f, nil
}

func arg(args []any, i int) any {
	if i >= len(args) {
		return nil
	}

	return args[i]
}

// timeArg returns a javascript time value. QuickJS tags a small integral one as
// an int, so both cases have to be handled.
func timeArg(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func unmarshalArg(args []any, i int, v any) error {
	s, ok := arg(args, i).(string)
	if !ok {
		return errors.New("internal error: expected a json argument")
	}

	return json.Unmarshal([]byte(s), v)
}

// resolved returns f, the resolved state of a formatter, as the json string the
// shim keeps and hands back on every format call.
func resolved(f any) (any, error) { return marshal(f) }

// formatted returns parts either as the json formatToParts result or, joined,
// as the format one. The format result crosses back as itself rather than
// wrapped in json: it is the hot path, and a JSON.parse of it in the shim
// costs about as much as the call.
func formatted(parts []part, wantParts bool) (any, error) {
	if wantParts {
		return marshal(parts)
	}

	var b strings.Builder
	for _, v := range parts {
		b.WriteString(v.Value)
	}
	return b.String(), nil
}

// failure returns the string form telling the shim which javascript error to
// throw. errorMark cannot occur in anything a formatter produces.
func failure(err error) (any, error) {
	e := &jsError{ctor: "Error", msg: err.Error()}
	errors.As(err, &e)
	return errorMark + e.ctor + errorMark + e.msg, nil
}

// errorMark is the first byte of a host function result that is an error
// rather than a value, and the separator between the error type and message.
const errorMark = "\x01"

func marshal(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		// Nothing here can fail to marshal, but a Go error is still better than
		// handing the shim something it cannot parse.
		return nil, err
	}

	return string(b), nil
}
