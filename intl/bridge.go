// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/liustack/quickjs"
)

// The bridge between intl.js and the formatters.
//
// Each Intl constructor is a ctor: its options, in the order javascript
// reads them, and how Go resolves them into a formatter. intl.js reads and
// converts the options, Go validates them, keeps the formatter in the
// registry (registry.go) and hands javascript its id, which every other call
// passes first. Calls go through Builtins, which convert nothing: a resolve
// takes strings, numbers and booleans, a format a number or a string, and
// they return a string, a number, an array of strings (VM.NewArray) or
// undefined. resolvedOptions(), an object, crosses as json for JSON.parse,
// which builds it faster than javascript would and calls no setter of the
// page. formatToParts() crosses as the strings of its parts.
//
// # Adding a constructor
//
// The steps for Intl.ListFormat, say, or a Temporal type:
//
//  1. The formatter: a type with the resolved state as json fields (fields
//     only Go reads start with an underscore and stay out of
//     resolvedOptions), an init method compiling what formatting needs, and
//     a resolve function from a request. Once init has run, it is read only:
//     every VM of the process shares it.
//  2. A ctor for it here, with its options in the order ECMA-402 reads them,
//     and its entry in ctors. resolveBuiltin(c) is its constructor's
//     Builtin.
//  3. Builtins for its methods, appended to bridge. Each takes the id first
//     and starts with formatterOf, returning undefined when that says the
//     registry dropped the formatter. Errors are jsErr(err): a rangeError or
//     a typeError is thrown as that javascript error.
//  4. In intl.js, a parameter for each new Builtin, in bridge order, and the
//     constructor: a function declaration whose parameters all have
//     defaults, so that its length is 0, which resolves through resolve()
//     with pushLocales and pushOptions and keeps the result with new
//     Slots(this, ...). defineConstructor(C, statics, methods) gives it its
//     prototype: methods and getters written as methods of an object
//     literal, each starting with internal(this, kind) and calling the
//     Builtins through call(), which resolves a dropped formatter again.
//  5. Nothing in intl.js may use a global or a method a page can change:
//     only what P, from stub.js, holds, object literals, null-prototype
//     arrays (bare) and descriptors ({__proto__: null, ...}), apply for
//     calls, +v and `${v}` for conversions. A built-in it needs and P lacks
//     goes into P in stub.js. TestTamper and TestBuiltinShape check against
//     Node, in testdata/node: add the new constructor's uses there.
//
// # Formatters from internal/gointl
//
// A go-intl formatter, such as a *gointl.NumberFormat or a *gointl.Collator,
// is the kind of formatter the registry wants: once made it never changes and
// any number of goroutines may use it at once. So its formatter type here
// holds the resolved state as json fields, as in step 1, and makes the go-intl
// object in init, from the resolved locale and options, which keeps the
// registry the only cache: two requests that resolve alike share one go-intl
// object across all VMs. A few rules on top of the steps above:
//
//   - The default locale and time zone come from the request, as _zone,
//     the zone Register hands intl.js, does for DateTimeFormat, never from
//     gointl.HostLocale or gointl.HostTimeZone, which read the machine the
//     process runs on.
//   - Strings go to javascript through VM.NewString or stringArray, which copy
//     them into the VM. go-intl may hand out strings and bytes that sit in the
//     read only data of its packs: never write to them, and copy bytes
//     (string(b)) before keeping them past the call.
//   - A result that is a number, as Collator.compare's, is VM.NewInt or
//     VM.NewFloat64. Parts go through formatted, a gointl.Part as
//     part{Type: string(p.Kind), Value: p.Value}.
//   - Data loaded for the whole process, such as Temporal's
//     temporal.LoadData, which takes milliseconds, is loaded once (a
//     sync.OnceValues), not per formatter.

// An option is one property of a constructor's options argument.
type option struct {
	name string
	// kind is the conversion javascript applies before passing it on: 's'
	// ToString, 'n' ToNumber, 'b' ToBoolean and 'g' a boolean kept as it is,
	// anything else ToString (useGrouping).
	kind byte
}

// A ctor describes an Intl constructor to the bridge.
type ctor struct {
	name string // as in Intl.NumberFormat
	tag  byte   // the first byte of its request keys
	svc  service
	// fixed names the leading string arguments of a resolve, which come
	// from intl.js rather than from the page, such as the VM's default time
	// zone. They go into the options with these names.
	fixed   []string
	options []option
	resolve func(request) (formatter, error)
}

var (
	numberFormatCtor = &ctor{
		name: "NumberFormat",
		tag:  'N',
		svc:  numberService,
		options: append(stringOptions("localeMatcher", "style", "currency", "currencyDisplay", "currencySign", "unit",
			"unitDisplay", "numberingSystem", "notation", "compactDisplay", "signDisplay", "roundingMode",
			"trailingZeroDisplay"),
			append(digitOptions(), option{"useGrouping", 'g'})...),
		resolve: func(req request) (formatter, error) { return resolveNumberFormat(req) },
	}

	dateTimeFormatCtor = &ctor{
		name:  "DateTimeFormat",
		tag:   'D',
		svc:   dateService,
		fixed: []string{"_zone", "_required", "_defaults"},
		options: append(stringOptions("localeMatcher", "calendar", "numberingSystem", "timeZone", "hourCycle",
			"weekday", "era", "year", "month", "day", "dayPeriod", "hour", "minute", "second", "timeZoneName",
			"formatMatcher", "dateStyle", "timeStyle"),
			option{"hour12", 'b'}, option{"fractionalSecondDigits", 'n'}),
		resolve: func(req request) (formatter, error) {
			zone, _ := req.Options["_zone"].(string)
			delete(req.Options, "_zone")
			return resolveDateTimeFormat(req, zone)
		},
	}

	pluralRulesCtor = &ctor{
		name:    "PluralRules",
		tag:     'P',
		svc:     pluralService,
		options: append(stringOptions("localeMatcher", "type", "notation", "roundingMode", "roundingPriority", "trailingZeroDisplay"), digitOptions()...),
		resolve: func(req request) (formatter, error) { return resolvePluralRules(req) },
	}

	ctors = []*ctor{numberFormatCtor, dateTimeFormatCtor, pluralRulesCtor}
)

func stringOptions(names ...string) []option {
	r := make([]option, len(names))
	for i, n := range names {
		r[i] = option{n, 's'}
	}
	return r
}

func digitOptions() []option {
	return []option{
		{"minimumIntegerDigits", 'n'}, {"minimumFractionDigits", 'n'}, {"maximumFractionDigits", 'n'},
		{"minimumSignificantDigits", 'n'}, {"maximumSignificantDigits", 'n'},
	}
}

// optionLists tells intl.js the options of each constructor, as json: the
// name of the constructor to the names of its options and a string of their
// kinds, one byte each.
var optionLists = func() string {
	type list struct {
		Names []string `json:"names"`
		Kinds string   `json:"kinds"`
	}
	m := map[string]list{}
	for _, c := range ctors {
		l := list{Names: []string{}}
		for _, o := range c.options {
			l.Names = append(l.Names, o.name)
			l.Kinds += string(o.kind)
		}
		m[c.name] = l
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}

	return string(b)
}()

// The arguments of a resolve are c.fixed, then the locales, then c.options.
// The locales are two arguments: a single tag, when the page passed a string,
// or else the list of tags (see splitLocales).

// key appends the request key of the arguments a, which tells two requests
// apart exactly when the arguments differ.
func (c *ctor) key(b []byte, a quickjs.Args) ([]byte, error) {
	b = append(b, c.tag)
	n := len(c.fixed) + 2 + len(c.options)
	for i := range n {
		if s, ok := a.String(i); ok {
			b = append(b, 's')
			b = strconv.AppendInt(b, int64(len(s)), 10)
			b = append(b, ':')
			b = append(b, s...)
			continue
		}

		if f, ok := a.Number(i); ok {
			b = append(b, 'n')
			b = strconv.AppendUint(b, math.Float64bits(f), 16)
			b = append(b, ';')
			continue
		}

		switch v, ok := a.Bool(i); {
		case !ok:
			if !a.Value(i).IsUndefined() {
				return nil, errors.New("intl: internal error: an argument of " + c.name + " has the wrong type")
			}

			b = append(b, 'u')
		case v:
			b = append(b, 't')
		default:
			b = append(b, 'f')
		}
	}
	return b, nil
}

// request returns the request the arguments a make.
func (c *ctor) request(a quickjs.Args) (request, error) {
	req := request{Options: map[string]any{}}
	for i, name := range c.fixed {
		s, _ := a.String(i)
		req.Options[name] = s
	}
	n := len(c.fixed)
	if s, ok := a.String(n); ok {
		req.Locales = []string{s}
	} else if s, ok := a.String(n + 1); ok {
		l, ok := splitLocales(s)
		if !ok {
			return request{}, errors.New("intl: internal error: a malformed locale list")
		}

		req.Locales = l
	}
	n += 2
	for i, o := range c.options {
		j := n + i
		switch o.kind {
		case 's':
			if s, ok := a.String(j); ok {
				req.Options[o.name] = s
			}
		case 'n':
			if f, ok := a.Number(j); ok {
				req.Options[o.name] = f
			}
		case 'b':
			if v, ok := a.Bool(j); ok {
				req.Options[o.name] = v
			}
		case 'g':
			if v, ok := a.Bool(j); ok {
				req.Options[o.name] = v
			} else if s, ok := a.String(j); ok {
				req.Options[o.name] = s
			}
		}
	}
	return req, nil
}

// splitLocales parses the list of tags javascript passes as one string: each
// tag preceded by its length, in UTF-16 code units as javascript counts them,
// and a colon, "5:en-US2:de".
func splitLocales(s string) ([]string, bool) {
	r := []string{}
	for s != "" {
		i := 0
		n := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			n = 10*n + int(s[i]-'0')
			if n > len(s) {
				return nil, false
			}

			i++
		}
		if i == 0 || i == len(s) || s[i] != ':' {
			return nil, false
		}

		s = s[i+1:]
		j := 0
		for n > 0 {
			if j == len(s) {
				return nil, false
			}

			c, size := utf8.DecodeRuneInString(s[j:])
			j += size
			n -= utf16.RuneLen(c)
		}
		if n < 0 {
			return nil, false
		}

		r = append(r, s[:j])
		s = s[j:]
	}
	return r, true
}

// resolveBuiltin returns the Builtin resolving the arguments of a new c into
// the id of its formatter.
func resolveBuiltin(c *ctor) *quickjs.Builtin {
	return quickjs.NewBuiltin(c.name, 0, func(a quickjs.Args) (quickjs.Value, error) {
		var buf [256]byte
		key, err := c.key(buf[:0], a)
		if err != nil {
			return quickjs.Value{}, err
		}

		e := formatters.lookup(key)
		if e == nil {
			req, err := c.request(a)
			if err != nil {
				return quickjs.Value{}, err
			}

			f, err := c.resolve(req)
			if err != nil {
				return quickjs.Value{}, jsErr(err)
			}

			if e, err = formatters.add(key, f); err != nil {
				return quickjs.Value{}, jsErr(err)
			}
		}
		// Ids go past 2^31, which an int of 32 bits does not hold.
		return a.VM().NewFloat64(float64(e.id)), nil
	})
}

// formatterOf returns the formatter whose id is argument 0 of a, and false
// when the registry has dropped it: javascript then resolves it again and
// repeats the call.
func formatterOf[T formatter](a quickjs.Args) (f T, ok bool, err error) {
	id, _ := a.Number(0)
	e := formatters.entry(id)
	if e == nil {
		return f, false, nil
	}

	if f, ok = e.f.(T); !ok {
		return f, false, errors.New("intl: internal error: a formatter of the wrong type")
	}

	return f, true, nil
}

// numberArg returns argument i of a, a number or a string of digits, as
// numberValue takes it.
func numberArg(a quickjs.Args, i int) any {
	if f, ok := a.Number(i); ok {
		// An integer goes on as one, as the engine tags most of them, which
		// is faster to format and exact.
		if f == math.Trunc(f) && math.Abs(f) <= 1<<53 && (f != 0 || !math.Signbit(f)) {
			return int64(f)
		}

		return f
	}

	if s, ok := a.String(i); ok {
		return s
	}

	return nil
}

// The Builtins intl.js is made with, in the order of its parameters.
var bridge = []*quickjs.Builtin{
	resolveBuiltin(numberFormatCtor),
	quickjs.NewBuiltin("format", 2, func(a quickjs.Args) (quickjs.Value, error) {
		return numberFormatted(a, false)
	}),
	quickjs.NewBuiltin("formatToParts", 2, func(a quickjs.Args) (quickjs.Value, error) {
		return numberFormatted(a, true)
	}),
	resolveBuiltin(dateTimeFormatCtor),
	quickjs.NewBuiltin("format", 2, func(a quickjs.Args) (quickjs.Value, error) {
		return dateFormatted(a, false)
	}),
	quickjs.NewBuiltin("formatToParts", 2, func(a quickjs.Args) (quickjs.Value, error) {
		return dateFormatted(a, true)
	}),
	resolveBuiltin(pluralRulesCtor),
	quickjs.NewBuiltin("select", 2, func(a quickjs.Args) (quickjs.Value, error) {
		p, ok, err := formatterOf[*pluralFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		return a.VM().NewString(p.selectNumber(numberArg(a, 1)))
	}),
	quickjs.NewBuiltin("selectRange", 3, func(a quickjs.Args) (quickjs.Value, error) {
		p, ok, err := formatterOf[*pluralFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		return a.VM().NewString(p.selectRange(numberArg(a, 1), numberArg(a, 2)))
	}),
	quickjs.NewBuiltin("resolvedOptions", 1, func(a quickjs.Args) (quickjs.Value, error) {
		id, _ := a.Number(0)
		e := formatters.entry(id)
		if e == nil {
			return quickjs.UndefinedValue, nil
		}

		return a.VM().NewString(e.options)
	}),
	quickjs.NewBuiltin("getCanonicalLocales", 2, func(a quickjs.Args) (quickjs.Value, error) {
		tags, err := localesArg(a, 0)
		if err != nil {
			return quickjs.Value{}, err
		}

		r := []string{}
		for _, tag := range tags {
			s, err := canonicalLocale(tag)
			if err != nil {
				return quickjs.Value{}, jsErr(err)
			}

			if !contains(r, s) {
				r = append(r, s)
			}
		}
		return stringArray(a.VM(), r)
	}),
	quickjs.NewBuiltin("supportedLocalesOf", 3, func(a quickjs.Args) (quickjs.Value, error) {
		name, _ := a.String(0)
		var c *ctor
		for _, v := range ctors {
			if v.name == name {
				c = v
			}
		}
		if c == nil {
			return quickjs.Value{}, errors.New("intl: internal error: no constructor " + name)
		}

		tags, err := localesArg(a, 1)
		if err != nil {
			return quickjs.Value{}, err
		}

		r, err := supportedLocales(tags, c.svc)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		return stringArray(a.VM(), r)
	}),
}

// localesArg returns the locales arguments i and i+1 of a.
func localesArg(a quickjs.Args, i int) ([]string, error) {
	if s, ok := a.String(i); ok {
		return []string{s}, nil
	}

	s, ok := a.String(i + 1)
	if !ok {
		return nil, nil
	}

	l, ok := splitLocales(s)
	if !ok {
		return nil, errors.New("intl: internal error: a malformed locale list")
	}

	return l, nil
}

func numberFormatted(a quickjs.Args, wantParts bool) (quickjs.Value, error) {
	f, ok, err := formatterOf[*numberFormat](a)
	if !ok {
		return quickjs.UndefinedValue, err
	}

	parts, err := f.format(numberArg(a, 1))
	if err != nil {
		return quickjs.Value{}, jsErr(err)
	}

	return formatted(a.VM(), parts, wantParts)
}

func dateFormatted(a quickjs.Args, wantParts bool) (quickjs.Value, error) {
	f, ok, err := formatterOf[*dateTimeFormat](a)
	if !ok {
		return quickjs.UndefinedValue, err
	}

	ms, _ := a.Number(1)
	parts, err := f.format(ms)
	if err != nil {
		return quickjs.Value{}, jsErr(err)
	}

	return formatted(a.VM(), parts, wantParts)
}

// formatted returns parts either as the array of their types and values, in
// turn, which parts() in intl.js makes the formatToParts result of, or,
// joined, as the format one.
func formatted(vm *quickjs.VM, parts []part, wantParts bool) (quickjs.Value, error) {
	if wantParts {
		l := make([]string, 0, 2*len(parts))
		for _, p := range parts {
			l = append(l, p.Type, p.Value)
		}
		return stringArray(vm, l)
	}

	if len(parts) == 1 {
		return vm.NewString(parts[0].Value)
	}

	n := 0
	for _, p := range parts {
		n += len(p.Value)
	}
	b := make([]byte, 0, n)
	for _, p := range parts {
		b = append(b, p.Value...)
	}
	return vm.NewString(string(b))
}

// stringArray returns a javascript array of l.
func stringArray(vm *quickjs.VM, l []string) (quickjs.Value, error) {
	vals := make([]quickjs.Value, 0, len(l))
	free := func() {
		for i := range vals {
			vals[i].Free()
		}
	}
	for _, s := range l {
		v, err := vm.NewString(s)
		if err != nil {
			free()
			return quickjs.Value{}, err
		}

		vals = append(vals, v)
	}
	a, err := vm.NewArray(vals...)
	free()
	return a, err
}

// jsErr returns err as the javascript error ECMA-402 has for it: a jsError's
// own, else an Error.
func jsErr(err error) error {
	var e *jsError
	if errors.As(err, &e) {
		return &quickjs.Error{Name: e.ctor, Message: e.msg}
	}

	return &quickjs.Error{Name: "Error", Message: err.Error()}
}
