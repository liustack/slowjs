//go:build gointl_all

package intl_test

import (
	"slices"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// heapSource answers with the same writable copy of the embedded data each
// time, as a Source that keeps its files in memory may.
type heapSource map[string][]byte

func (s heapSource) Open(m intl.Marker, d intl.DataLocale) ([]byte, error) {
	key := string(m) + "/" + d.String()
	if b, ok := s[key]; ok {
		return b, nil
	}
	b, err := intl.Embedded.Open(m, d)
	if err != nil {
		return nil, err
	}
	b = slices.Clone(b)
	s[key] = b
	return b, nil
}

// A ZoneRecord is the caller's own: writing to its TransitionTypes changes
// neither the Source nor the next record. They had been the Source's memory,
// which for the embedded data is read-only, so that a write was a fault no
// recover catches (ISSUES.md API-9).
func TestZoneRecordIsACopy(t *testing.T) {
	src := heapSource{}
	first, err := intl.LoadZoneRecord(src, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Clone(first.TransitionTypes)
	if len(want) == 0 {
		t.Fatal("America/New_York has no transitions")
	}
	for i := range first.TransitionTypes {
		first.TransitionTypes[i] ^= 0xff
	}
	first.Transitions[0]++
	second, err := intl.LoadZoneRecord(src, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(second.TransitionTypes, want) {
		t.Errorf("TransitionTypes changed through an earlier record")
	}
	if second.Transitions[0] == first.Transitions[0] {
		t.Errorf("Transitions changed through an earlier record")
	}

	embedded, err := intl.LoadZoneRecord(intl.Embedded, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	embedded.TransitionTypes[0] ^= 0xff // had faulted
}
