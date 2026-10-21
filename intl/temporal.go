// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"encoding/json"
	"math"
	"runtime"
	"strconv"
	"sync"
	"weak"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
)

// The toLocaleString of the Temporal types, for package temporal: ECMA-402's
// CreateDateTimeFormat with the required and default components each type
// asks for, and the format the DateTimeFormat keeps for that type
// (GetDateTimeFormat and AdjustDateTimeStyleFormat), resolved here from the
// same options through resolveDateTimeFormat. A plain value is formatted at
// UTC, a PlainDate at noon and a PlainTime on 1970-01-01, as ECMA-402 has
// it.

func init() {
	intlhook.Format = temporalFormatBuiltin
	intlhook.DefaultZone = defaultZoneOf
	l := struct {
		Names []string `json:"names"`
		Kinds string   `json:"kinds"`
	}{Names: []string{}}
	for _, o := range dateTimeFormatCtor.options {
		l.Names = append(l.Names, o.name)
		l.Kinds += string(o.kind)
	}
	b, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}
	intlhook.Options = string(b)
}

// vmZones are the default zones of the VMs Register was given one for.
var vmZones sync.Map // weak.Pointer[quickjs.VM] -> string

func rememberZone(vm *quickjs.VM, name string) {
	if name == "UTC" {
		return
	}
	key := weak.Make(vm)
	vmZones.Store(key, name)
	runtime.AddCleanup(vm, func(k weak.Pointer[quickjs.VM]) { vmZones.Delete(k) }, key)
}

func defaultZoneOf(vm *quickjs.VM) string {
	if z, ok := vmZones.Load(weak.Make(vm)); ok {
		return z.(string)
	}
	return "UTC"
}

// A temporalKind is how a Temporal type is formatted: the required and
// default components of its toLocaleString, and those of its format.
type temporalKind struct {
	name               string
	required, defaults string // of CreateDateTimeFormat
	// fRequired, fDefaults and inherit are GetDateTimeFormat's arguments
	// for the format of the type.
	fRequired, fDefaults string
	inherit              bool // all, rather than relevant
	// allowed are the components AdjustDateTimeStyleFormat keeps, nil for
	// the style's own format; style says which style the format needs.
	allowed []string
	style   string // "date", "time", or "" for either
	plain   bool
	// anyCalendar says the value's calendar may be iso8601 too, rather
	// than only the formatter's.
	anyCalendar bool
}

var (
	dateComponents = []string{"weekday", "era", "year", "month", "day"}
	timeComponents = []string{"dayPeriod", "hour", "minute", "second", "fractionalSecondDigits"}
	// anyComponents are those GetDateTimeFormat tells present by.
	anyComponents = []string{"weekday", "year", "month", "day", "dayPeriod", "hour", "minute", "second",
		"fractionalSecondDigits"}
	allComponents = []string{"weekday", "era", "year", "month", "day", "dayPeriod", "hour", "minute", "second",
		"fractionalSecondDigits", "timeZoneName"}

	temporalKinds = map[string]*temporalKind{
		"date": {name: "PlainDate", required: "date", defaults: "date", fRequired: "date", fDefaults: "date",
			allowed: dateComponents, style: "date", plain: true, anyCalendar: true},
		"yearMonth": {name: "PlainYearMonth", required: "date", defaults: "date", fRequired: "year-month",
			fDefaults: "year-month", allowed: []string{"era", "year", "month"}, style: "date", plain: true},
		"monthDay": {name: "PlainMonthDay", required: "date", defaults: "date", fRequired: "month-day",
			fDefaults: "month-day", allowed: []string{"month", "day"}, style: "date", plain: true},
		"time": {name: "PlainTime", required: "time", defaults: "time", fRequired: "time", fDefaults: "time",
			allowed: timeComponents, style: "time", plain: true, anyCalendar: true},
		"dateTime": {name: "PlainDateTime", required: "any", defaults: "all", fRequired: "any", fDefaults: "all",
			allowed: append(append([]string{}, dateComponents...), timeComponents...), plain: true, anyCalendar: true},
		"instant": {name: "Instant", required: "any", defaults: "all", fRequired: "any", fDefaults: "all",
			inherit: true, anyCalendar: true},
		"zoned": {name: "ZonedDateTime", required: "any", defaults: "all", fRequired: "any",
			fDefaults: "zoned-date-time", inherit: true, anyCalendar: true},
	}
)

func requiredComponents(required string) []string {
	switch required {
	case "date":
		return []string{"weekday", "year", "month", "day"}
	case "time":
		return timeComponents
	case "year-month":
		return []string{"year", "month"}
	case "month-day":
		return []string{"month", "day"}
	}
	return anyComponents
}

func defaultComponents(defaults string) []string {
	switch defaults {
	case "date":
		return []string{"year", "month", "day"}
	case "time":
		return []string{"hour", "minute", "second"}
	case "year-month":
		return []string{"year", "month"}
	case "month-day":
		return []string{"month", "day"}
	}
	return []string{"year", "month", "day", "hour", "minute", "second"}
}

// temporalFormat is a type's format resolved from a request: nil where
// the DateTimeFormat has none for it, and the error of a request
// CreateDateTimeFormat rejects.
type temporalFormat struct {
	f        *dateTimeFormat
	calendar string // the DateTimeFormat's
	err      error
}

var (
	temporalFormatsMu sync.Mutex
	temporalFormats   = map[string]*temporalFormat{}
)

// maxTemporalFormats bounds the formats kept: more are dropped all at once,
// as a page making up options in a loop would otherwise grow the map.
const maxTemporalFormats = 512

func copyOptions(o map[string]any) map[string]any {
	r := make(map[string]any, len(o)+3)
	for k, v := range o {
		r[k] = v
	}
	return r
}

func isComponent(name string) bool { return contains(allComponents, name) }

// resolveTemporal resolves the format of kind k for req, as
// CreateDateTimeFormat and then GetDateTimeFormat or
// AdjustDateTimeStyleFormat do. zone is a ZonedDateTime's zone, "" for the
// other kinds.
func resolveTemporal(k *temporalKind, req request, defaultZone, zone string) *temporalFormat {
	o := req.Options
	if k.name == "ZonedDateTime" {
		if _, ok := o["timeZone"]; ok {
			return &temporalFormat{err: typeError("Invalid time zone specified: %s", zone)}
		}
		o = copyOptions(o)
		o["timeZone"] = zone
	}
	mainOptions := copyOptions(o)
	mainOptions["_required"] = k.required
	mainOptions["_defaults"] = k.defaults
	main, err := resolveDateTimeFormat(request{Locales: req.Locales, Options: mainOptions}, defaultZone)
	if err != nil {
		return &temporalFormat{err: err}
	}

	r := &temporalFormat{calendar: main.Calendar}
	// The options that are no component or style, which every format of
	// the DateTimeFormat shares.
	base := map[string]any{}
	for name, v := range o {
		if !isComponent(name) && name != "dateStyle" && name != "timeStyle" {
			base[name] = v
		}
	}
	if k.plain {
		base["timeZone"] = "UTC"
	}

	var opts map[string]any
	if main.DateStyle != "" || main.TimeStyle != "" {
		switch {
		case k.style == "date" && main.DateStyle == "", k.style == "time" && main.TimeStyle == "":
			return r
		case k.allowed == nil:
			r.f = main
			return r
		}
		// AdjustDateTimeStyleFormat: the style's format if it has only
		// components the type has, else the closest one that has.
		got := &dateTimeFormat{Pattern: main.Pattern}
		got.readComponents()
		values := map[string]string{"weekday": got.Weekday, "era": got.Era, "year": got.Year, "month": got.Month,
			"day": got.Day, "dayPeriod": got.DayPeriod, "hour": got.Hour, "minute": got.Minute, "second": got.Second,
			"timeZoneName": got.TimeZoneName}
		if got.FractionalSecondDigits != 0 {
			values["fractionalSecondDigits"] = strconv.Itoa(got.FractionalSecondDigits)
		}
		conflict := false
		for _, c := range allComponents {
			if values[c] != "" && !contains(k.allowed, c) {
				conflict = true
			}
		}
		if !conflict && !k.plain {
			r.f = main
			return r
		}
		opts = copyOptions(base)
		if conflict {
			for _, c := range k.allowed {
				switch v := values[c]; {
				case v == "":
				case c == "fractionalSecondDigits":
					n, _ := strconv.Atoi(v)
					opts[c] = float64(n)
				default:
					opts[c] = v
				}
			}
		} else {
			opts["dateStyle"], opts["timeStyle"] = o["dateStyle"], o["timeStyle"]
			for _, s := range []string{"dateStyle", "timeStyle"} {
				if opts[s] == nil {
					delete(opts, s)
				}
			}
		}
	} else {
		// GetDateTimeFormat.
		opts = copyOptions(base)
		if k.inherit {
			for _, c := range allComponents {
				if v, ok := o[c]; ok {
					opts[c] = v
				}
			}
		} else if v, ok := o["era"]; ok && (k.fRequired == "date" || k.fRequired == "year-month" || k.fRequired == "any") {
			opts["era"] = v
		}
		anyPresent := false
		for _, c := range anyComponents {
			if _, ok := o[c]; ok {
				anyPresent = true
			}
		}
		needDefaults := true
		for _, c := range requiredComponents(k.fRequired) {
			if v, ok := o[c]; ok {
				opts[c] = v
				needDefaults = false
			}
		}
		if needDefaults {
			if anyPresent && !k.inherit {
				return r
			}
			for _, c := range defaultComponents(k.fDefaults) {
				opts[c] = "numeric"
			}
			if _, ok := opts["timeZoneName"]; !ok && k.fDefaults == "zoned-date-time" {
				opts["timeZoneName"] = "short"
			}
		}
	}
	f, err := resolveDateTimeFormat(request{Locales: req.Locales, Options: opts}, defaultZone)
	if err != nil {
		return &temporalFormat{err: err}
	}
	if err := f.init(); err != nil {
		return &temporalFormat{err: err}
	}
	r.f = f
	return r
}

// The arguments of temporalFormatBuiltin, see intlhook.Format.
const (
	tfKind = iota
	tfCalendar
	tfTime
	tfZone
	tfOffset
	tfLocales
	tfOptions = tfLocales + 2
)

var temporalFormatBuiltin = quickjs.NewBuiltin("toLocaleString", 0, func(a quickjs.Args) (quickjs.Value, error) {
	kindName, _ := a.String(tfKind)
	k := temporalKinds[kindName]
	if k == nil {
		return quickjs.Value{}, &quickjs.Error{Name: "Error", Message: "intl: internal error: no Temporal kind " + kindName}
	}
	cal, _ := a.String(tfCalendar)
	ms, _ := a.Number(tfTime)
	zone, _ := a.String(tfZone)
	defaultZone := defaultZoneOf(a.VM())

	key := []byte(kindName)
	key = append(key, 0)
	key = append(key, defaultZone...)
	key = append(key, 0)
	key = append(key, zone...)
	key = append(key, 0)
	opts := dateTimeFormatCtor.options
	for i := tfLocales; i < tfOptions+len(opts); i++ {
		switch {
		case !a.Value(i).IsUndefined():
			if s, ok := a.String(i); ok {
				key = append(key, 's')
				key = strconv.AppendInt(key, int64(len(s)), 10)
				key = append(key, ':')
				key = append(key, s...)
			} else if f, ok := a.Number(i); ok {
				key = append(key, 'n')
				key = strconv.AppendUint(key, math.Float64bits(f), 16)
				key = append(key, ';')
			} else if v, ok := a.Bool(i); ok && v {
				key = append(key, 't')
			} else if ok {
				key = append(key, 'f')
			} else {
				return quickjs.Value{}, &quickjs.Error{Name: "Error", Message: "intl: internal error: an argument of toLocaleString has the wrong type"}
			}
		default:
			key = append(key, 'u')
		}
	}

	temporalFormatsMu.Lock()
	tf := temporalFormats[string(key)]
	temporalFormatsMu.Unlock()
	if tf == nil {
		req := request{Options: map[string]any{}}
		if s, ok := a.String(tfLocales); ok {
			req.Locales = []string{s}
		} else if s, ok := a.String(tfLocales + 1); ok {
			l, ok := splitLocales(s)
			if !ok {
				return quickjs.Value{}, &quickjs.Error{Name: "Error", Message: "intl: internal error: a malformed locale list"}
			}
			req.Locales = l
		}
		for i, o := range opts {
			j := tfOptions + i
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
			}
		}
		tf = resolveTemporal(k, req, defaultZone, zone)
		temporalFormatsMu.Lock()
		if len(temporalFormats) >= maxTemporalFormats {
			temporalFormats = map[string]*temporalFormat{}
		}
		temporalFormats[string(key)] = tf
		temporalFormatsMu.Unlock()
	}

	if tf.err != nil {
		return quickjs.Value{}, jsErr(tf.err)
	}
	if cal != tf.calendar && !(k.anyCalendar && cal == "iso8601") {
		return quickjs.Value{}, &quickjs.Error{Name: "RangeError", Message: "Mismatched calendars."}
	}
	if tf.f == nil {
		return quickjs.Value{}, &quickjs.Error{Name: "TypeError", Message: "Invalid argument for Temporal [object Temporal." + k.name + "]"}
	}
	if k.name == "ZonedDateTime" {
		// The offset Temporal computed, so that toString and toLocaleString
		// of a value agree where Go's zone data and Temporal's differ.
		off, _ := a.Number(tfOffset)
		return formatted(a.VM(), tf.f.f.formatAt(ms, int(off/1e9)), false)
	}
	return formatted(a.VM(), tf.f.f.format(ms), false)
})
