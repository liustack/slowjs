//go:build gointl_all

package tzdata

import (
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, z := range []*Zone{
		{Name: "Asia/Calcutta", Canonical: "Asia/Calcutta", Link: "asia/kolkata"},
		{
			Name: "Europe/Paris", Canonical: "Europe/Paris",
			Types:      []Offset{{561, 0}, {0, 0}, {0, 3600}, {3600, 0}, {-18000, 3600}},
			Trans:      []int64{-1855958961, -1689814800, 1 << 40},
			TransTypes: []uint8{1, 2, 4},
			Final:      []int32{3600, 1997, 2, -31, -1, 3600, 2, 9, -31, -1, 3600, 2, 3600},
		},
		{Name: "Etc/GMT+5", Canonical: "Etc/GMT+5", Types: []Offset{{-18000, 0}}},
	} {
		got, err := Decode(Encode(z))
		if err != nil {
			t.Fatalf("%s: %v", z.Name, err)
		}
		if !reflect.DeepEqual(got, z) {
			t.Errorf("%s: read back %+v, want %+v", z.Name, got, z)
		}
	}
}

func TestDecodeRefusesMismatchedTransitions(t *testing.T) {
	// Two transitions, and the type of only one of them.
	b := Encode(&Zone{Name: "X", Types: []Offset{{0, 0}}, Trans: []int64{1, 2}, TransTypes: []uint8{0}})
	if z, err := Decode(b); err == nil {
		t.Errorf("read %+v from transitions with too few types", z)
	}
}
