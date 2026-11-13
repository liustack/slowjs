// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strconv"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/intlhook"
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

// listItems splits the items intl.js sends as one string, in WTF-8 as
// Args.WTF8 reads it: each is its length in UTF-16 code units, a colon and
// the item. Each item is the bytes of s it takes, not a copy: go-intl reads
// only the first characters of an item, and javascript writes the items
// itself (listParts). A long list stops at the eval timeout of vm (see
// checkTurn), and a long item between pieces of its units (unitsEnd).
func listItems(vm *quickjs.VM, s string) ([]string, bool) {
	var items []string
	for i := 0; i < len(s); {
		checkTurn(vm, len(items))
		j := i
		for j < len(s) && j-i <= maxLengthDigits && s[j] != ':' {
			j++
		}
		n, err := strconv.Atoi(s[i:j])
		if err != nil || j == len(s) || s[j] != ':' {
			return nil, false
		}
		end, ok := unitsEnd(vm, s, j+1, n)
		if !ok {
			return nil, false
		}

		items = append(items, s[j+1:end])
		i = end
	}
	return items, true
}

// maxLengthDigits is the most digits the length of an item is written with:
// a string has fewer than 2³² units.
const maxLengthDigits = 10

// unitsEnd is the byte offset of s after n UTF-16 code units from byte i,
// with the call checked between pieces of intlhook.Piece units: a lone
// surrogate, three bytes in WTF-8, is one unit, and a character past the BMP,
// four bytes, two. It is false when s ends first.
func unitsEnd(vm *quickjs.VM, s string, i, n int) (int, bool) {
	for n > 0 {
		intlhook.Check(vm)
		for piece := min(n, intlhook.Piece); piece > 0; {
			if i >= len(s) {
				return 0, false
			}
			c := s[i]
			switch {
			case c < 0x80:
				i++
			case c < 0xe0:
				i += 2
			case c < 0xf0:
				i += 3
			default:
				if n == 1 {
					// The item ends inside the pair of a character.
					return 0, false
				}
				i += 4
				piece--
				n--
			}
			piece--
			n--
		}
	}
	return i, i <= len(s)
}

// listParts is the list joined, for javascript to put together: the text
// between the items as strings, and each item as its index in items, so
// that javascript writes the item it has, lone surrogates and all. An item
// in a part that is not one of the items as they come, in order, which no
// locale's patterns make, is written as go-intl has it.
func listParts(vm *quickjs.VM, items []string, parts []gointl.ListPart) (quickjs.Value, error) {
	vals := make([]quickjs.Value, 0, len(parts))
	free := func() {
		for i := range vals {
			vals[i].Free()
		}
	}
	next := 0
	for i, p := range parts {
		if stopTurn(vm, i) {
			free()
			return quickjs.UndefinedValue, nil
		}
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

		v, err := intlhook.Emit(vm, p.Value)
		if err != nil {
			free()
			return quickjs.Value{}, err
		}

		vals = append(vals, v)
	}
	a, err := intlhook.EmitValues(vm, vals)
	free()
	return a, err
}
