//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

func BenchmarkNewDateTimeFormat(b *testing.B) {
	benchmarkNewDateTimeFormat(b, intl.ZoneNone)
}

// BenchmarkNewDateTimeFormatZone is the cost of reading the zone names, which
// a formatter does only when its pattern writes a zone.
func BenchmarkNewDateTimeFormatZone(b *testing.B) {
	benchmarkNewDateTimeFormat(b, intl.ZoneLongGeneric)
}

func benchmarkNewDateTimeFormat(b *testing.B, zone intl.ZoneStyle) {
	loc, err := intl.ParseLocale("de")
	if err != nil {
		b.Fatal(err)
	}
	opts := intl.DateTimeFormatOptions{TimeZone: "Europe/London", Hour: intl.WidthNumeric, TimeZoneName: zone}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := intl.NewDateTimeFormat(loc, opts); err != nil {
			b.Fatal(err)
		}
	}
}
