//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The private use before "lvariant" goes with it, as ICU's ultag_parse
// drops it: "en-x-foo-lvariant-abcde" is "en-abcde", where go-intl had kept
// "-x-foo". Node's answers (ISSUES.md LO-6).
func TestLvariantDropsPrivateUse(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"en-x-foo-lvariant-abcde", "en-abcde"},
		{"en-x-foo-bar-lvariant-abcde", "en-abcde"},
		{"en-x-lvariant-abcde", "en-abcde"},
		{"en-x-foo", "en-x-foo"},
		{"en-US-x-foo-lvariant-posix", "en-US-u-va-posix"},
		{"en-u-ca-roc-x-foo-lvariant-abcde", "en-abcde-u-ca-roc"},
		{"en-x-foo-lvariant-abcde-fonipa", "en-abcde-fonipa"},
	} {
		got, err := canon.Canonicalize(c.tag)
		if err != nil || got.String() != c.want {
			t.Errorf("%s: %v, %v; want %s", c.tag, got, err, c.want)
		}
	}
}
