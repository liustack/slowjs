// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package intl implements ECMA-402, the ECMAScript Internationalization API,
// for a [quickjs.VM].
//
// QuickJS has no Intl. It is not a disabled build option: neither Bellard's
// engine nor quickjs-ng implements ECMA-402, so a page or bundle that reaches
// for Intl.DateTimeFormat gets a ReferenceError. [Register] installs a Go
// backed implementation of the two parts of the API code actually depends on:
//
//   - Intl.DateTimeFormat, with the IANA time zone database behind its
//     timeZone option.
//   - Intl.NumberFormat, with grouping, fraction and significant digits,
//     percent, currency, unit, and compact and scientific notation.
//
// Register also replaces Date.prototype.toLocaleString, toLocaleDateString and
// toLocaleTimeString along with Number.prototype.toLocaleString, which ECMA-402
// redefines in terms of those two constructors. QuickJS ships fallbacks for
// them that ignore their arguments, so
//
//	new Date(0).toLocaleString("en-US", {timeZone: "America/New_York"})
//
// answers in the host's own zone before Register - a wrong time, silently -
// and "12/31/1969, 7:00:00 PM" after it.
//
// # Locale data
//
// Go's standard library ships the time zone database but no CLDR, so the
// locale data here is en-US and only en-US. Any locale argument is accepted
// and resolves to en-US, which resolvedOptions().locale then reports instead of
// echoing what was asked for. Within en-US the output is meant to match a
// browser exactly. Where the data does not exist the code falls back to
// something it can derive rather than to a guess:
//
//   - A long time zone name formats as the GMT offset, "GMT+01:00", since
//     CLDR's "Eastern Standard Time" is not available. UTC is spelled out.
//   - currencyDisplay "name" formats as the currency code.
//   - A currency with no symbol here formats as its code and a unit with no
//     name as its identifier, which are CLDR's own fallbacks.
//
// Not implemented: Intl.Collator, Intl.PluralRules, Intl.RelativeTimeFormat,
// Intl.ListFormat, Intl.DisplayNames, Intl.Segmenter, Intl.DurationFormat and
// Intl.supportedValuesOf. String.prototype.localeCompare keeps QuickJS's code
// point ordering. NumberFormat accepts roundingIncrement and roundingPriority
// but only implements their defaults, which is what resolvedOptions reports.
//
// # Differences from a browser
//
// The en-US output is checked expression by expression against a full ICU -
// node's Intl - over a few thousand combinations of options, values, instants
// and time zones by TestAgainstICU, which runs whenever node is installed. It
// agrees except where the data is missing (above) or where ICU disagrees with
// itself:
//
//   - The space before AM or PM is an ordinary one. V8 writes U+202F there in
//     formatToParts but an ordinary space in format, and one separator has to
//     be picked for both.
//   - hourCycle "h23" is reported for hour12: false. V8 reports "h24" while
//     still formatting midnight as 00, which is the value ECMA-402 asks for.
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

// script is javascript compiled to bytecode once per process, by the first VM
// that needs it, and evaluated from there on every VM: the bytecode does not
// depend on the VM that made it.
type script struct {
	src  *string
	once sync.Once
	code []byte
	err  error
}

func (s *script) eval(vm *quickjs.VM) error {
	s.once.Do(func() { s.code, s.err = vm.Compile(*s.src, quickjs.EvalGlobal) })
	if s.err != nil {
		return s.err
	}

	_, err := vm.EvalBytecode(s.code)
	return err
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
