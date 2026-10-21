// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
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
//  2. A ctor for it here, with its options in the order ECMA-402 reads them
//     (see the note on numberFormatCtor for how the checks differ),
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
//     calls, +v and toString(v) for conversions: not a template literal,
//     which QuickJS compiles into a call of String.prototype.concat. A
//     built-in it needs and P lacks
//     goes into P in stub.js. TestTamper and TestBuiltinShape check against
//     Chromium, in testdata/chromium: add the new constructor's uses to
//     tamper.js and shape.js there and record them again (record.cjs).
//
// # Objects that are not formatters
//
// An Intl.Locale is a tag and what its getters answer, which localeNew works
// out once and javascript keeps in the object's slots. Nothing of it goes in
// the registry: its methods pass the tag to localeInfo, which answers from
// go-intl's LocaleInfo, and Intl.supportedValuesOf answers lists made once
// per process.
//
// # Formatters from internal/gointl
//
// A go-intl formatter, such as a *gointl.NumberFormat or a *gointl.Collator,
// is the kind of formatter the registry wants: once made it never changes and
// any number of goroutines may use it at once. So its formatter type here
// holds the resolved state as json fields, as in step 1, and keeps the go-intl
// object in an unexported field, which keeps the registry the only cache: two
// requests that resolve alike share one go-intl object across all VMs. A few
// rules on top of the steps above:
//
//   - go-intl resolves the locale as well (gointl.go): the ctor names its
//     gointl.Service in goSvc, which supportedLocalesOf answers from too, and
//     resolve makes the go-intl object, so init has nothing left to do.
//     resolvedOptions is the go-intl object's, in the json fields.
//   - Strings from javascript are Args.String, with U+FFFD for each lone
//     surrogate, or Args.WTF8 where a lone surrogate must stay itself, as it
//     does in Collator.compare.
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
	// goSvc selects the go-intl locale set for this constructor.
	goSvc gointl.Service
	// fixed names the leading string arguments of a resolve, which come
	// from intl.js rather than from the page, such as the VM's default time
	// zone. They go into the options with these names.
	fixed   []string
	options []option
	resolve func(request) (formatter, error)
}

var (
	// The options are in the order ECMA-402 reads them, which test262
	// checks. Where ECMA-402 checks each as it reads it, so that a bad
	// option stops the reading, intl.js reads and converts them all and Go
	// checks them after, in the order it reads them: the same options throw
	// the same errors, only the getters of later options run too. And
	// ECMA-402 converts the digit options of SetNumberFormatDigitOptions
	// once it has read them all, where intl.js converts each as it reads it.
	numberFormatCtor = &ctor{
		name:  "NumberFormat",
		tag:   'N',
		goSvc: gointl.ServiceNumberFormat,
		options: concat(
			stringOptions("localeMatcher", "numberingSystem", "style", "currency", "currencyDisplay", "currencySign",
				"unit", "unitDisplay", "notation"),
			digitOptions(),
			stringOptions("compactDisplay"), []option{{"useGrouping", 'g'}}, stringOptions("signDisplay")),
		resolve: func(req request) (formatter, error) { return resolveNumberFormat(req) },
	}

	dateTimeFormatCtor = &ctor{
		name:  "DateTimeFormat",
		tag:   'D',
		goSvc: gointl.ServiceDateTimeFormat,
		fixed: []string{"_zone", "_required", "_defaults"},
		options: concat(
			stringOptions("localeMatcher", "calendar", "numberingSystem"), []option{{"hour12", 'b'}},
			stringOptions("hourCycle", "timeZone", "weekday", "era", "year", "month", "day", "dayPeriod", "hour",
				"minute", "second"),
			[]option{{"fractionalSecondDigits", 'n'}},
			stringOptions("timeZoneName", "formatMatcher", "dateStyle", "timeStyle")),
		resolve: func(req request) (formatter, error) {
			zone, _ := req.Options["_zone"].(string)
			delete(req.Options, "_zone")
			return resolveDateTimeFormat(req, zone)
		},
	}

	pluralRulesCtor = &ctor{
		name:    "PluralRules",
		tag:     'P',
		goSvc:   gointl.ServicePluralRules,
		options: concat(stringOptions("localeMatcher", "type", "notation", "compactDisplay"), digitOptions()),
		resolve: func(req request) (formatter, error) { return resolvePluralRules(req) },
	}

	collatorCtor = &ctor{
		name:  "Collator",
		tag:   'C',
		goSvc: gointl.ServiceCollator,
		options: []option{{"usage", 's'}, {"localeMatcher", 's'}, {"collation", 's'}, {"numeric", 'b'},
			{"caseFirst", 's'}, {"sensitivity", 's'}, {"ignorePunctuation", 'b'}},
		resolve: func(req request) (formatter, error) { return resolveCollator(req) },
	}

	segmenterCtor = &ctor{
		name:    "Segmenter",
		tag:     'S',
		goSvc:   gointl.ServiceSegmenter,
		options: stringOptions("localeMatcher", "granularity"),
		resolve: func(req request) (formatter, error) { return resolveSegmenter(req) },
	}

	relativeTimeFormatCtor = &ctor{
		name:    "RelativeTimeFormat",
		tag:     'R',
		goSvc:   gointl.ServiceRelativeTimeFormat,
		options: stringOptions("localeMatcher", "numberingSystem", "style", "numeric"),
		resolve: func(req request) (formatter, error) { return resolveRelativeTimeFormat(req) },
	}

	listFormatCtor = &ctor{
		name:    "ListFormat",
		tag:     'L',
		goSvc:   gointl.ServiceListFormat,
		options: stringOptions("localeMatcher", "type", "style"),
		resolve: func(req request) (formatter, error) { return resolveListFormat(req) },
	}

	displayNamesCtor = &ctor{
		name:    "DisplayNames",
		tag:     'I',
		goSvc:   gointl.ServiceDisplayNames,
		options: stringOptions("localeMatcher", "style", "type", "fallback", "languageDisplay"),
		resolve: func(req request) (formatter, error) { return resolveDisplayNames(req) },
	}

	durationFormatCtor = &ctor{
		name:    "DurationFormat",
		tag:     'U',
		goSvc:   gointl.ServiceDurationFormat,
		options: concat(stringOptions("localeMatcher", "numberingSystem", "style"), durationOptions(), []option{{"fractionalDigits", 'n'}}),
		resolve: func(req request) (formatter, error) { return resolveDurationFormat(req) },
	}

	ctors = []*ctor{numberFormatCtor, dateTimeFormatCtor, pluralRulesCtor, collatorCtor, segmenterCtor,
		relativeTimeFormatCtor, listFormatCtor, displayNamesCtor, durationFormatCtor}
)

func stringOptions(names ...string) []option {
	r := make([]option, len(names))
	for i, n := range names {
		r[i] = option{n, 's'}
	}
	return r
}

// digitOptions are those SetNumberFormatDigitOptions reads.
func digitOptions() []option {
	return []option{
		{"minimumIntegerDigits", 'n'}, {"minimumFractionDigits", 'n'}, {"maximumFractionDigits", 'n'},
		{"minimumSignificantDigits", 'n'}, {"maximumSignificantDigits", 'n'}, {"roundingIncrement", 'n'},
		{"roundingMode", 's'}, {"roundingPriority", 's'}, {"trailingZeroDisplay", 's'},
	}
}

// durationOptions are those GetDurationUnitOptions reads, each unit's style
// and display in turn.
func durationOptions() []option {
	var r []option
	for _, u := range durationUnits {
		r = append(r, option{u, 's'}, option{u + "Display", 's'})
	}
	return r
}

func concat(lists ...[]option) []option {
	var r []option
	for _, l := range lists {
		r = append(r, l...)
	}
	return r
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

// numberArg returns argument i of a, a number or a numeric string.
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
	quickjs.NewBuiltin("format", 3, func(a quickjs.Args) (quickjs.Value, error) {
		return numberFormatted(a, false)
	}),
	quickjs.NewBuiltin("formatToParts", 3, func(a quickjs.Args) (quickjs.Value, error) {
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

		r, err := goSupported(c.goSvc, tags)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		return stringArray(a.VM(), r)
	}),
	resolveBuiltin(collatorCtor),
	quickjs.NewBuiltin("compare", 3, func(a quickjs.Args) (quickjs.Value, error) {
		c, ok, err := formatterOf[*collator](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		// go-intl weighs a lone surrogate by its code point, as ICU does, so
		// the strings keep theirs.
		x, _ := a.WTF8(1)
		y, _ := a.WTF8(2)
		return a.VM().NewInt(c.c.Compare(x, y)), nil
	}),
	resolveBuiltin(segmenterCtor),
	quickjs.NewBuiltin("breaks", 2, func(a quickjs.Args) (quickjs.Value, error) {
		s, ok, err := formatterOf[*segmenter](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		text, _ := a.WTF8(1)
		return intArray(a.VM(), s.breaks(text))
	}),
	resolveBuiltin(relativeTimeFormatCtor),
	// rtfFormat and rtfParts take the value, a number, and the unit, a
	// string. The parts are their type, value and unit in turn, the unit
	// empty for the text around the number.
	quickjs.NewBuiltin("rtfFormat", 3, func(a quickjs.Args) (quickjs.Value, error) {
		f, ok, err := formatterOf[*relativeTimeFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		v, _ := a.Number(1)
		u, _ := a.String(2)
		unit, err := f.unit(v, u)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		return a.VM().NewString(f.f.Format(v, unit))
	}),
	quickjs.NewBuiltin("rtfParts", 3, func(a quickjs.Args) (quickjs.Value, error) {
		f, ok, err := formatterOf[*relativeTimeFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		v, _ := a.Number(1)
		u, _ := a.String(2)
		unit, err := f.unit(v, u)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		parts := f.f.FormatToParts(v, unit)
		l := make([]string, 0, 3*len(parts))
		for _, p := range parts {
			l = append(l, string(p.Kind), p.Value, p.Unit)
		}
		return stringArray(a.VM(), l)
	}),
	resolveBuiltin(listFormatCtor),
	// lfParts takes the items as listItems reads them and answers as
	// listParts writes them.
	quickjs.NewBuiltin("lfParts", 2, func(a quickjs.Args) (quickjs.Value, error) {
		f, ok, err := formatterOf[*listFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		s, _ := a.WTF8(1)
		items, ok := listItems(s)
		if !ok {
			return quickjs.Value{}, typeError("malformed list")
		}

		return listParts(a.VM(), items, f.f.FormatToParts(items))
	}),
	resolveBuiltin(displayNamesCtor),
	// dnOf answers the name, or the number 0 for none.
	quickjs.NewBuiltin("dnOf", 2, func(a quickjs.Args) (quickjs.Value, error) {
		d, ok, err := formatterOf[*displayNames](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		code, _ := a.String(1)
		name, ok, err := d.of(code)
		switch {
		case err != nil:
			return quickjs.Value{}, jsErr(err)
		case !ok:
			return a.VM().NewInt(0), nil
		}
		return a.VM().NewString(name)
	}),
	resolveBuiltin(durationFormatCtor),
	// dfFormat and dfParts take the duration as durationOf does: the
	// string, and true for fields. The parts are their type, value and
	// unit in turn, the unit empty for the list's own text.
	durationFormatBuiltin,
	quickjs.NewBuiltin("dfParts", 3, func(a quickjs.Args) (quickjs.Value, error) {
		f, ok, err := formatterOf[*durationFormat](a)
		if !ok {
			return quickjs.UndefinedValue, err
		}

		s, _ := a.WTF8(1)
		fields, _ := a.Bool(2)
		d, err := durationOf(s, fields)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		parts, err := f.f.FormatToParts(d)
		if err != nil {
			return quickjs.Value{}, jsErr(rangeError("%s", err))
		}

		l := make([]string, 0, 3*len(parts))
		for _, p := range parts {
			l = append(l, string(p.Kind), p.Value, p.Unit)
		}
		return stringArray(a.VM(), l)
	}),
	// supportedValuesOf answers a new array of the list of the key.
	quickjs.NewBuiltin("supportedValuesOf", 1, func(a quickjs.Args) (quickjs.Value, error) {
		key, _ := a.String(0)
		values, err := supportedValues()
		if err != nil {
			return quickjs.Value{}, err
		}

		l, ok := values[key]
		if !ok {
			return quickjs.Value{}, jsErr(rangeError("Invalid key : %s", key))
		}

		return stringArray(a.VM(), l)
	}),
	// localeCheck throws the RangeError of Intl.Locale for its tag or one of
	// the options checkLocale checks.
	quickjs.NewBuiltin("localeCheck", 2, func(a quickjs.Args) (quickjs.Value, error) {
		kind, _ := a.String(0)
		s, _ := a.String(1)
		if err := checkLocale(kind, s); err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		return quickjs.UndefinedValue, nil
	}),
	// localeNew takes the tag and the options of Intl.Locale, each a string
	// or undefined, and answers the json of the new Locale's slots.
	quickjs.NewBuiltin("localeNew", 1+locOptions, func(a quickjs.Args) (quickjs.Value, error) {
		tag, _ := a.String(0)
		var opts [locOptions]*string
		for i := range opts {
			if s, ok := a.String(1 + i); ok {
				opts[i] = &s
			}
		}
		r, err := newLocale(tag, &opts)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		s, err := localeJSON(r)
		if err != nil {
			return quickjs.Value{}, err
		}

		return a.VM().NewString(s)
	}),
	// localeInfo answers what the method of Intl.Locale named by its second
	// argument answers for the tag of a Locale, its first: maximize and
	// minimize the tag of the Locale to make.
	quickjs.NewBuiltin("localeInfo", 2, func(a quickjs.Args) (quickjs.Value, error) {
		tag, _ := a.String(0)
		what, _ := a.String(1)
		return localeInfo(a.VM(), tag, what)
	}),
	// caseLanguage answers the language whose case rules toLocaleUpperCase
	// and toLocaleLowerCase follow for a tag, or "" for the default ones.
	quickjs.NewBuiltin("caseLanguage", 1, func(a quickjs.Args) (quickjs.Value, error) {
		tag, _ := a.String(0)
		lang, err := caseLanguage(tag)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
		}

		return a.VM().NewString(lang)
	}),
	// casePremap takes a string, a language from caseLanguage and whether to
	// map to upper case, and answers what the language's rules make of the
	// string, for the engine to map the rest: a string, or an array of
	// strings and the code units of lone surrogates; undefined where they
	// change nothing.
	quickjs.NewBuiltin("casePremap", 3, func(a quickjs.Args) (quickjs.Value, error) {
		s, _ := a.WTF8(0)
		lang, _ := a.String(1)
		upper, _ := a.Bool(2)
		pieces, ok, err := premapCase(s, lang, upper)
		switch {
		case err != nil:
			return quickjs.Value{}, err
		case !ok:
			return quickjs.UndefinedValue, nil
		case len(pieces) == 1:
			if s, ok := pieces[0].(string); ok {
				return a.VM().NewString(s)
			}
		}
		return mixedArray(a.VM(), pieces)
	}),
	resolveBuiltin(temporalDateFormatCtor),
	quickjs.NewBuiltin("temporalFormat", 3, func(a quickjs.Args) (quickjs.Value, error) { return temporalDateFormatted(a, false, false) }),
	quickjs.NewBuiltin("temporalParts", 3, func(a quickjs.Args) (quickjs.Value, error) { return temporalDateFormatted(a, true, false) }),
	quickjs.NewBuiltin("temporalRange", 5, func(a quickjs.Args) (quickjs.Value, error) { return temporalDateFormatted(a, false, true) }),
	quickjs.NewBuiltin("temporalRangeParts", 5, func(a quickjs.Args) (quickjs.Value, error) { return temporalDateFormatted(a, true, true) }),
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

	v := numberArg(a, 1)
	if isBigInt, _ := a.Bool(2); isBigInt {
		v = bigintDecimal(v.(string))
	}
	parts := f.format(v)

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

// mixedArray is l, strings and ints, as a javascript array.
func mixedArray(vm *quickjs.VM, l []any) (quickjs.Value, error) {
	vals := make([]quickjs.Value, 0, len(l))
	free := func() {
		for i := range vals {
			vals[i].Free()
		}
	}
	for _, x := range l {
		var v quickjs.Value
		switch x := x.(type) {
		case string:
			var err error
			if v, err = vm.NewString(x); err != nil {
				free()
				return quickjs.Value{}, err
			}
		case int:
			v = vm.NewInt(x)
		default:
			free()
			return quickjs.Value{}, errors.New("intl: internal error: a value of another type in an array")
		}
		vals = append(vals, v)
	}
	a, err := vm.NewArray(vals...)
	free()
	return a, err
}

// intArray is l as a javascript array of numbers.
func intArray(vm *quickjs.VM, l []int) (quickjs.Value, error) {
	vals := make([]quickjs.Value, len(l))
	for i, n := range l {
		vals[i] = vm.NewInt(n)
	}
	return vm.NewArray(vals...)
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

var durationFormatBuiltin = quickjs.NewBuiltin("dfFormat", 3, func(a quickjs.Args) (quickjs.Value, error) {
	f, ok, err := formatterOf[*durationFormat](a)
	if !ok {
		return quickjs.UndefinedValue, err
	}

	s, _ := a.WTF8(1)
	fields, _ := a.Bool(2)
	d, err := durationOf(s, fields)
	if err != nil {
		return quickjs.Value{}, jsErr(err)
	}

	r, err := f.f.Format(d)
	if err != nil {
		return quickjs.Value{}, jsErr(rangeError("%s", err))
	}

	return a.VM().NewString(r)
})
