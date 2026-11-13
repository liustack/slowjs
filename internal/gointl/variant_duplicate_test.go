//go:build gointl_all

package intl_test

import (
	"errors"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A variant alias that names a variant the locale already has leaves it
// once, and a variant given again by -x-lvariant- is refused, as ICU's
// parser refuses any variant given twice. go-intl had written
// "en-alalc97-alalc97" and "en-fonipa-fonipa". Node's answers (ISSUES.md
// LO-2).
func TestVariantGivenTwice(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"en-heploc-alalc97", "en-alalc97"},
		{"en-alalc97-heploc", "en-alalc97"},
		{"ja-hepburn-heploc", "ja-alalc97"},
		{"en-polytoni-heploc", "en-alalc97-polyton"},
		{"en-x-lvariant-fonipa", "en-fonipa"},
		{"en-fonipa-x-lvariant-fonipa", ""},
		{"en-heploc-heploc", ""},
	} {
		got, err := canon.Canonicalize(c.tag)
		if c.want == "" {
			if !errors.Is(err, intl.ErrSyntax) {
				t.Errorf("%s: %v, %v; want ErrSyntax", c.tag, got, err)
			}
			continue
		}
		if err != nil || got.String() != c.want {
			t.Errorf("%s: %v, %v; want %s", c.tag, got, err, c.want)
		}
	}
}
