// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"testing"
	"time"
)

// A zone whose CLDR name the host's zoneinfo lacks, as a system without
// tzdata-legacy lacks Asia/Calcutta and Europe/Kiev, is loaded under another
// of its names, IANA's, and keeps its CLDR name.
func TestZoneUnderAnotherName(t *testing.T) {
	defer func(f func(string) (*time.Location, error)) { loadLocation = f }(loadLocation)
	loadLocation = func(name string) (*time.Location, error) {
		if name == "Asia/Calcutta" || name == "Europe/Kiev" {
			return nil, errors.New("unknown time zone " + name)
		}
		return time.LoadLocation(name)
	}
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		in, name string
		offset   int
	}{
		{"Asia/Calcutta", "Asia/Calcutta", 19800},
		{"asia/kolkata", "Asia/Calcutta", 19800},
		{"Europe/Kiev", "Europe/Kiev", 7200},
		{"Europe/Kyiv", "Europe/Kiev", 7200},
	} {
		name, loc, err := resolveTimeZone(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.in, err)
			continue
		}
		if _, off := at.In(loc).Zone(); name != test.name || off != test.offset {
			t.Errorf("%s: %s at %d s, want %s at %d s", test.in, name, off, test.name, test.offset)
		}
	}
}
