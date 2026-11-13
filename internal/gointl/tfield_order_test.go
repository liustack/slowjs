//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A "-t-" extension's fields are ordered by key and then by value, as ICU
// orders them; go-intl had ordered them by key alone, keeping the input's
// order for a key given twice. Node's answers (ISSUES.md LO-8).
func TestTransformedFieldOrder(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"art-CS-t-m0-names-names-m0-hwidth-names", "art-RS-t-m0-hwidth-names-m0-names-names"},
		{"und-t-m0-zzz-m0-aaa", "und-t-m0-aaa-m0-zzz"},
		{"und-t-m0-names-d0-ascii", "und-t-d0-ascii-m0-prprname"},
		{"und-t-s0-ascii-d0-fwidth", "und-t-d0-fwidth-s0-ascii"},
		{"und-t-k0-abc-k0-abc", "und-t-k0-abc-k0-abc"},
	} {
		got, err := canon.Canonicalize(c.tag)
		if err != nil || got.String() != c.want {
			t.Errorf("%s: %v, %v; want %s", c.tag, got, err, c.want)
		}
	}
}
