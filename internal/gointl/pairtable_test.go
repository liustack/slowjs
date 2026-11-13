//go:build gointl_all

package intl

import (
	"testing"

	"github.com/liustack/quickjs/internal/gointl/internal/layout"
)

// A table of locale pairs begins with its version, and one without it, as
// the generators wrote them before, is refused rather than misread
// (ISSUES.md RU-6).
func TestPairTableVersion(t *testing.T) {
	l, err := ParseLocale("en-AG")
	if err != nil {
		t.Fatal(err)
	}
	en := l.Data()
	key, _ := en.MarshalBinary()
	root, _ := DataLocale{}.MarshalBinary()
	record := append(append([]byte(nil), key...), root...)

	table, err := newPairTable(append([]byte{layout.Pairs}, record...))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := table.lookup(en); !ok || !got.IsRoot() {
		t.Errorf("lookup(en-AG) = %v, %v", got, ok)
	}
	for name, b := range map[string][]byte{
		"unversioned": record,
		"version 99":  append([]byte{99}, record...),
		"empty":       nil,
	} {
		if _, err := newPairTable(b); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	for _, m := range []Marker{MarkerLikelySubtags, MarkerParentLocales} {
		b, err := Embedded.Open(m, DataLocale{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := newPairTable(b); err != nil {
			t.Errorf("%s: %v", m, err)
		}
	}
}
