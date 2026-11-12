// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
)

// The original options, including omitted fields, distinguish Temporal
// patterns that have the same resolved legacy pattern. Keep them in the
// registry key so default fields are chosen per type. The key holds the
// SHA-256 of the request's json, not the json: a request has the locales
// and options as the page gave them, which can be long, as a list of many
// locales or a calendar option, any type sequence, is, and the registry
// keeps its keys for as long as the process runs.
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
		sum := sha256.Sum256(key)
		return &temporalDateFormat{Request: hex.EncodeToString(sum[:]), f: f}, nil
	},
}

func temporalDateFormatted(a quickjs.Args, wantParts, isRange bool) (quickjs.Value, error) {
	r, ok, err := formatterOf[*temporalDateFormat](a)
	if !ok {
		return quickjs.UndefinedValue, err
	}
	f := r.f
	door := intlhook.Open(a)
	cal, _ := door.Short(2, intlhook.KindTemporal)
	if err := f.checkCalendar(cal); err != nil {
		return quickjs.Value{}, jsErr(err)
	}
	ms, _ := a.Number(1)
	start, err := f.instant(ms)
	if err != nil {
		return quickjs.Value{}, jsErr(err)
	}
	if isRange {
		cal, _ = door.Short(4, intlhook.KindTemporal)
		if err := f.checkCalendar(cal); err != nil {
			return quickjs.Value{}, jsErr(err)
		}
		endMS, _ := a.Number(3)
		end, err := f.instant(endMS)
		if err != nil {
			return quickjs.Value{}, jsErr(err)
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
			return intlhook.Emit(a.VM(), text.String())
		}
		return intlhook.EmitStrings(a.VM(), list)
	}
	if !wantParts {
		return intlhook.Emit(a.VM(), plainSpaces.Replace(f.f.Format(start)))
	}
	parts := f.f.FormatToParts(start)
	list := make([]string, 0, 2*len(parts))
	for _, p := range parts {
		list = append(list, string(p.Kind), plainSpaces.Replace(p.Value))
	}
	return intlhook.EmitStrings(a.VM(), list)
}
