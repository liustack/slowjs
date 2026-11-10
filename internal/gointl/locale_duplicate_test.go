//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Of a keyword given twice the first is meant, however many keywords there
// are. Keywords were sorted with sort.Slice, which is not stable past twelve
// elements, so a long identifier could keep the second. Node's answers
// (ISSUES.md LO-1).
func TestDuplicateKeywordFirstWins(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"en-u-ca-gregory-co-phonebk-cu-usd-fw-mon-hc-h23-ka-shifted-kb-kc-kf-upper-kn-kr-space-ks-level1-ca-buddhist",
			"en-u-ca-gregory-co-phonebk-cu-usd-fw-mon-hc-h23-ka-shifted-kb-kc-kf-upper-kn-kr-space-ks-level1"},
		{"en-u-ks-level1-kr-space-kn-kf-upper-kc-kb-ka-shifted-hc-h23-fw-mon-cu-usd-co-phonebk-ca-gregory-ca-buddhist",
			"en-u-ca-gregory-co-phonebk-cu-usd-fw-mon-hc-h23-ka-shifted-kb-kc-kf-upper-kn-kr-space-ks-level1"},
		{"en-u-ca-gregory-ca-buddhist", "en-u-ca-gregory"},
		{"en-u-nu-latn-ca-roc-nu-arab", "en-u-ca-roc-nu-latn"},
	} {
		got, err := canon.Canonicalize(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != c.want {
			t.Errorf("%s: %s, want %s", c.tag, got, c.want)
		}
	}
}
