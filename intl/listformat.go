// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	gointl "github.com/liustack/slowjs/internal/gointl"
)

// listFormat is a resolved Intl.ListFormat.
type listFormat struct {
	Locale string `json:"locale"`
	Type   string `json:"type"`
	Style  string `json:"style"`

	f *gointl.ListFormat
}

// resolveListFormat is the Intl.ListFormat constructor but for reading the
// options, which intl.js does.
func resolveListFormat(req request) (*listFormat, error) {
	o := req.Options
	opts := gointl.ListFormatOptions{Compat: goCompat}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	loc, err := goResolve(req.vm, gointl.ServiceListFormat, req.Locales)
	if err != nil {
		return nil, err
	}

	switch s, err := optString(o, "type", "conjunction", "disjunction", "unit"); {
	case err != nil:
		return nil, err
	case s == "disjunction":
		opts.Type = gointl.Disjunction
	case s == "unit":
		opts.Type = gointl.UnitList
	}
	switch s, err := optString(o, "style", "long", "short", "narrow"); {
	case err != nil:
		return nil, err
	case s == "short":
		opts.Style = gointl.ListShort
	case s == "narrow":
		opts.Style = gointl.ListNarrow
	}
	f, err := gointl.NewListFormatFrom(chromiumData{}, loc, opts)
	if err != nil {
		return nil, err
	}

	r := f.ResolvedOptions()
	return &listFormat{
		Locale: r.Locale,
		Type:   [...]string{"conjunction", "disjunction", "unit"}[r.Type],
		Style:  [...]string{"long", "short", "narrow"}[r.Style],
		f:      f,
	}, nil
}

// init has nothing to do: go-intl made the formatter while resolving it.
func (f *listFormat) init() error { return nil }
