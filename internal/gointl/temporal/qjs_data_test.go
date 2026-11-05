// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"fmt"
	"os"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// qjsTemporal reads Temporal's data from src and says the offsets of some
// zones and a date in the Chinese calendar.
func qjsTemporal(src intl.Source) (string, error) {
	d, err := LoadData(src)
	if err != nil {
		return "", err
	}
	chinese, err := d.Calendar("chinese")
	if err != nil {
		return "", err
	}
	var out string
	for _, c := range []struct {
		zone string
		at   time.Time
	}{
		{"Asia/Shanghai", time.Date(2024, 3, 5, 14, 4, 5, 0, time.UTC)},
		{"America/Chicago", time.Date(2024, 3, 5, 14, 4, 5, 0, time.UTC)},
		{"Australia/Sydney", time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)},
		// Past the transitions tzdb lists, the rule goes on.
		{"America/New_York", time.Date(2060, 7, 1, 0, 0, 0, 0, time.UTC)},
	} {
		tz, err := d.Zones.TimeZoneFromIdentifier([]byte(c.zone))
		if err != nil {
			return "", err
		}
		i, err := InstantFromEpochMilliseconds(c.at.UnixMilli())
		if err != nil {
			return "", err
		}
		z, err := NewZonedDateTimeFromInstant(i, tz, chinese)
		if err != nil {
			return "", err
		}
		p := z.ToPlainDate()
		out += fmt.Sprintf("%s %s %d-%s-%d; ", c.zone, z.Offset(), p.Year(), p.MonthCode(), p.Day())
	}
	return out, nil
}

func TestQJSTemporalData(t *testing.T) {
	got, err := qjsTemporal(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	want := "Asia/Shanghai +08:00 2024-M01-25; America/Chicago -06:00 2024-M01-25; Australia/Sydney +10:00 2024-M05-26; America/New_York -04:00 2060-M06-3; "
	if got != want {
		t.Errorf("%q, want %q", got, want)
	}
}

// With QJS_GOINTL_DATA naming go-intl's data directory, Temporal reads from
// our packs what it reads from go-intl's data.
func TestQJSTemporalMatchesUpstream(t *testing.T) {
	dir := os.Getenv("QJS_GOINTL_DATA")
	if dir == "" {
		t.Skip("QJS_GOINTL_DATA is not set")
	}
	want, err := qjsTemporal(intl.NewFS(os.DirFS(dir)))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := qjsTemporal(intl.Embedded); err != nil || got != want {
		t.Errorf("%q, %v; go-intl's data: %q", got, err, want)
	}
}
