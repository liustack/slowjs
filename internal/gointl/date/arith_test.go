//go:build gointl_all

package date

import (
	"math"
	"testing"
)

// MakeTime and MakeDate round each product before they add, as V8 does. An
// arm64 build fused a multiply and an add into one rounding, and wrote
// 34448384 for Date.UTC(1970, 0, 213503982336, 0, 0, 0,
// -18446744073709552000), where Node writes 34447360. The fused value is
// computed here too, so that the test says something on a machine that
// never fuses.
func TestMakeDateRoundsEachProduct(t *testing.T) {
	day := MakeDay(1970, 0, 213503982336)
	ms := MakeTime(0, 0, 0, -18446744073709552000)
	if got := MakeDate(day, ms); got != 34447360 {
		t.Errorf("MakeDate = %v, want 34447360", got)
	}
	if fused := math.FMA(day, msPerDay, ms); fused != 34448384 {
		t.Errorf("fused = %v, want 34448384, which the test tells apart", fused)
	}
}
