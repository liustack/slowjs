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

// The bridge between intl.js and the formatters. Each constructor is a ctor:
// its options, in the order javascript reads them, and how Go resolves them
// into a formatter. intl.js reads and converts the options, Go validates
// them, keeps the formatter in the registry and hands javascript its id.
// Every other call passes that id first.
//
// All calls go through Builtins, which convert nothing: a resolve takes
// strings, numbers and booleans as arguments, a format a number or a string
// and returns a string. Only results that are objects, resolvedOptions() and
// formatToParts(), cross as json, which the engine's JSON.parse builds faster
// than javascript would.

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
		return a.VM().NewString(string(appendStrings(nil, r)))
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

		return a.VM().NewString(string(appendStrings(nil, r)))
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

// formatted returns parts either as the json of the formatToParts result or,
// joined, as the format one.
func formatted(vm *quickjs.VM, parts []part, wantParts bool) (quickjs.Value, error) {
	if wantParts {
		b := make([]byte, 0, 32*len(parts))
		b = append(b, '[')
		for i, p := range parts {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"type":`...)
			b = appendString(b, p.Type)
			b = append(b, `,"value":`...)
			b = appendString(b, p.Value)
			b = append(b, '}')
		}
		b = append(b, ']')
		return vm.NewString(string(b))
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

// appendStrings appends the json array of l.
func appendStrings(b []byte, l []string) []byte {
	b = append(b, '[')
	for i, s := range l {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendString(b, s)
	}
	return append(b, ']')
}

// appendString appends the json string of s, which is valid UTF-8.
func appendString(b []byte, s string) []byte {
	const hex = "0123456789abcdef"
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b = append(b, '\\', c)
		case c < 0x20:
			b = append(b, '\\', 'u', '0', '0', hex[c>>4], hex[c&15])
		default:
			b = append(b, c)
		}
	}
	return append(b, '"')
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
