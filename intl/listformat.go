// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"strconv"
	"unicode/utf16"

	"github.com/liustack/slowjs"
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

	loc, err := goResolve(gointl.ServiceListFormat, req.Locales)
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

// listItems splits the items intl.js sends as one string, in WTF-8 as
// Args.WTF8 reads it: each is its length in UTF-16 code units, a colon and
// the item.
func listItems(s string) ([]string, bool) {
	u := utf16Of(s)
	var items []string
	for i := 0; i < len(u); {
		j := i
		for j < len(u) && u[j] != ':' {
			j++
		}
		n, err := strconv.Atoi(string(utf16.Decode(u[i:j])))
		if err != nil || j+1+n > len(u) {
			return nil, false
		}

		items = append(items, string(utf16.Decode(u[j+1:j+1+n])))
		i = j + 1 + n
	}
	return items, true
}

// listParts is the list joined, for javascript to put together: the text
// between the items as strings, and each item as its index in items, so
// that javascript writes the item it has, lone surrogates and all. An item
// in a part that is not one of the items as they come, in order, which no
// locale's patterns make, is written as go-intl has it.
func listParts(vm *slowjs.VM, items []string, parts []gointl.ListPart) (slowjs.Value, error) {
	vals := make([]slowjs.Value, 0, len(parts))
	free := func() {
		for i := range vals {
			vals[i].Free()
		}
	}
	next := 0
	for _, p := range parts {
		if p.Kind == gointl.ListElement {
			for next < len(items) && items[next] == "" && p.Value != "" {
				next++
			}
			if next < len(items) && items[next] == p.Value {
				vals = append(vals, vm.NewInt(next))
				next++
				continue
			}
		}

		v, err := vm.NewString(p.Value)
		if err != nil {
			free()
			return slowjs.Value{}, err
		}

		vals = append(vals, v)
	}
	a, err := vm.NewArray(vals...)
	free()
	return a, err
}
