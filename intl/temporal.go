// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strconv"
	"sync"
	"time"
	"weak"

	"github.com/liustack/slowjs"
	gointl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/intlhook"
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
	l.Names, l.Kinds = []string{}, ""
	for _, o := range durationFormatCtor.options {
		l.Names = append(l.Names, o.name)
		l.Kinds += string(o.kind)
	}
	b, err = json.Marshal(l)
	if err != nil {
		panic(err)
	}
	intlhook.DurationOptions = string(b)
	intlhook.DurationResolve = resolveBuiltin(durationFormatCtor)
	intlhook.DurationFormat = durationFormatBuiltin
}

// vmZones are the default zones of the VMs Register was given one for.
var vmZones sync.Map // weak.Pointer[slowjs.VM] -> string

func rememberZone(vm *slowjs.VM, name string) {
	if name == "UTC" {
		return
	}
	key := weak.Make(vm)
	vmZones.Store(key, name)
	runtime.AddCleanup(vm, func(k weak.Pointer[slowjs.VM]) { vmZones.Delete(k) }, key)
}

func defaultZoneOf(vm *slowjs.VM) string {
	if z, ok := vmZones.Load(weak.Make(vm)); ok {
		return z.(string)
	}
	return "UTC"
}

// A temporalKind is how a Temporal type is formatted: the required and
// default components of its toLocaleString, and those of its format.
type temporalKind struct {
	name               string
	required, defaults string
	value              gointl.TemporalKind
	plain              bool
}

var temporalKinds = map[string]*temporalKind{
	"legacy":    {name: "Date"},
	"date":      {"PlainDate", "date", "date", gointl.TemporalPlainDate, true},
	"time":      {"PlainTime", "time", "time", gointl.TemporalPlainTime, true},
	"dateTime":  {"PlainDateTime", "any", "all", gointl.TemporalPlainDateTime, true},
	"yearMonth": {"PlainYearMonth", "date", "date", gointl.TemporalPlainYearMonth, true},
	"monthDay":  {"PlainMonthDay", "date", "date", gointl.TemporalPlainMonthDay, true},
	"instant":   {"Instant", "any", "all", gointl.TemporalInstant, false},
	"zoned":     {"ZonedDateTime", "any", "all", gointl.TemporalInstant, false},
}

// temporalFormat is a type's format resolved from a request: nil where
// the DateTimeFormat has none for it, and the error of a request
// CreateDateTimeFormat rejects.
type temporalFormat struct {
	f    *gointl.DateTimeFormat
	base *gointl.DateTimeFormat
	kind *temporalKind
	err  error
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

// resolveTemporal builds the same type-specific formatter for public Intl
// methods and Temporal.toLocaleString. The legacy resolver validates the
// options and chooses the calendars this package supports. go-intl supplies
// V8's Temporal patterns and interval patterns from the same CLDR release.
func resolveTemporal(k *temporalKind, req request, defaultZone, zone string) *temporalFormat {
	o := copyOptions(req.Options)
	if k.name == "ZonedDateTime" {
		if _, ok := o["timeZone"]; ok {
			return &temporalFormat{err: typeError("Invalid time zone specified: %s", zone)}
		}
		o["timeZone"] = zone
		o["_temporalZone"] = true
	}
	main, err := resolveDateTimeFormat(request{Locales: req.Locales, Options: o}, defaultZone)
	if err != nil {
		return &temporalFormat{err: err}
	}
	base := main.f
	r := &temporalFormat{base: base, kind: k}
	r.f = base
	if k.value != 0 {
		r.f, err = base.ForTemporalChromium(k.value)
		if errors.Is(err, gointl.ErrTemporalFormat) {
			return r
		}
		r.err = err
	}
	return r
}

func stringOpt(o map[string]any, key string) string { s, _ := o[key].(string); return s }

func (f *temporalFormat) checkCalendar(cal string) error {
	if f.err != nil {
		return f.err
	}
	kind := f.kind.value
	if f.kind.name == "ZonedDateTime" {
		// Zoned values use instant formatting, but their calendar must
		// match by the same rule as PlainDateTime.
		kind = gointl.TemporalPlainDateTime
	}
	if kind != 0 && !f.base.CalendarMatches(kind, cal) {
		return rangeError("Mismatched calendars.")
	}
	if f.f == nil {
		return typeError("Invalid argument for Temporal [object Temporal.%s]", f.kind.name)
	}
	return nil
}

func (f *temporalFormat) instant(ms float64) (time.Time, error) {
	// TimeClip belongs to legacy Date values. Temporal constructors already
	// checked their own limits, which allow plain fields beyond Date's last
	// instant and a YearMonth's reference day before its first one.
	if math.IsNaN(ms) || math.IsInf(ms, 0) || f.kind.value == 0 && math.Abs(ms) > maxTimeValue {
		return time.Time{}, rangeError("Invalid time value")
	}
	t := time.UnixMilli(int64(ms)).UTC()
	if f.kind.plain {
		t = f.f.PlainInstant(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
	}
	return t, nil
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

var temporalFormatBuiltin = slowjs.NewBuiltin("toLocaleString", 0, func(a slowjs.Args) (slowjs.Value, error) {
	kindName, _ := a.String(tfKind)
	k := temporalKinds[kindName]
	if k == nil {
		return slowjs.Value{}, &slowjs.Error{Name: "Error", Message: "intl: internal error: no Temporal kind " + kindName}
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
				return slowjs.Value{}, &slowjs.Error{Name: "Error", Message: "intl: internal error: an argument of toLocaleString has the wrong type"}
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
				return slowjs.Value{}, &slowjs.Error{Name: "Error", Message: "intl: internal error: a malformed locale list"}
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
		req.Options["_required"], req.Options["_defaults"] = k.required, k.defaults
		tf = resolveTemporal(k, req, defaultZone, zone)
		temporalFormatsMu.Lock()
		if len(temporalFormats) >= maxTemporalFormats {
			temporalFormats = map[string]*temporalFormat{}
		}
		temporalFormats[string(key)] = tf
		temporalFormatsMu.Unlock()
	}

	if err := tf.checkCalendar(cal); err != nil {
		return slowjs.Value{}, jsErr(err)
	}
	t, err := tf.instant(ms)
	if err != nil {
		return slowjs.Value{}, jsErr(err)
	}
	if k.name == "ZonedDateTime" {
		offset, _ := a.Number(tfOffset)
		return a.VM().NewString(plainSpaces.Replace(tf.f.FormatTemporalZoned(t, int(offset/1e9))))
	}
	return a.VM().NewString(plainSpaces.Replace(tf.f.Format(t)))
})
