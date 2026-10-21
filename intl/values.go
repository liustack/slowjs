// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"slices"
	"sync"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// unlistedCollations are the collations go-intl's list names that no
// locale of its data has, which Chromium leaves out too: dict for si and
// phonetic for ln. TestSupportedCollations checks them.
var unlistedCollations = []string{"dict", "phonetic"}

// supportedValues are the lists Intl.supportedValuesOf answers with, by
// key: go-intl's, as V8 builds them from ICU, of the values the
// constructors here take. A calendar is listed when DateTimeFormat writes
// dates in it in en, as test262 asks, and so is a numbering system when
// NumberFormat has its digits.
var supportedValues = sync.OnceValues(func() (map[string][]string, error) {
	collations, err := gointl.Collations()
	if err != nil {
		return nil, err
	}

	currencies, err := gointl.Currencies()
	if err != nil {
		return nil, err
	}

	systems, err := gointl.NumberingSystems()
	if err != nil {
		return nil, err
	}

	zones, err := gointl.TimeZones(goCompat)
	if err != nil {
		return nil, err
	}

	calendars := gointl.Calendars(goCompat)
	collations = slices.DeleteFunc(collations, func(c string) bool { return slices.Contains(unlistedCollations, c) })
	units := slices.Sorted(slices.Values(sanctionedUnits))
	return map[string][]string{
		"calendar": calendars, "collation": collations, "currency": currencies, "numberingSystem": systems,
		"timeZone": zones, "unit": units,
	}, nil
})
