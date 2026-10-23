// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/liustack/slowjs"
)

// The original options, including omitted fields, distinguish Temporal
// patterns that have the same resolved legacy pattern. Keep them in the
// registry key so default fields are chosen per type.
type temporalDateFormat struct {
	Request string `json:"_request"`
	f       *temporalFormat
}

func (f *temporalDateFormat) init() error { return nil }

var temporalDateFormatCtor = &ctor{
	name: "TemporalDateTimeFormat", tag: 'T',
	fixed:   []string{"_kind", "_zone", "_required", "_defaults"},
	options: dateTimeFormatCtor.options,
	resolve: func(req request) (formatter, error) {
		k := temporalKinds[stringOpt(req.Options, "_kind")]
		if k == nil {
			return nil, errors.New("intl: unknown internal Temporal kind")
		}
		f := resolveTemporal(k, req, stringOpt(req.Options, "_zone"), "")
		if f.err != nil {
			return nil, f.err
		}
		key, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		return &temporalDateFormat{Request: string(key), f: f}, nil
	},
}

func temporalDateFormatted(a slowjs.Args, wantParts, isRange bool) (slowjs.Value, error) {
	r, ok, err := formatterOf[*temporalDateFormat](a)
	if !ok {
		return slowjs.UndefinedValue, err
	}
	f := r.f
	cal, _ := a.String(2)
	if err := f.checkCalendar(cal); err != nil {
		return slowjs.Value{}, jsErr(err)
	}
	ms, _ := a.Number(1)
	start, err := f.instant(ms)
	if err != nil {
		return slowjs.Value{}, jsErr(err)
	}
	if isRange {
		cal, _ = a.String(4)
		if err := f.checkCalendar(cal); err != nil {
			return slowjs.Value{}, jsErr(err)
		}
		endMS, _ := a.Number(3)
		end, err := f.instant(endMS)
		if err != nil {
			return slowjs.Value{}, jsErr(err)
		}
		parts := f.f.FormatRangeToParts(start, end)
		list := make([]string, 0, 3*len(parts))
		var text strings.Builder
		for _, p := range parts {
			value := plainSpaces.Replace(p.Value)
			list = append(list, string(p.Kind), value, p.Source.String())
			text.WriteString(value)
		}
		if !wantParts {
			return a.VM().NewString(text.String())
		}
		return stringArray(a.VM(), list)
	}
	if !wantParts {
		return a.VM().NewString(plainSpaces.Replace(f.f.Format(start)))
	}
	parts := f.f.FormatToParts(start)
	list := make([]string, 0, 2*len(parts))
	for _, p := range parts {
		list = append(list, string(p.Kind), plainSpaces.Replace(p.Value))
	}
	return stringArray(a.VM(), list)
}
