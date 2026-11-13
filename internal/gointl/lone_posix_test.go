//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A lone variant "posix" becomes "-u-va-posix" once the aliases are
// replaced, as ICU writes it when it makes the tag: "en-arevela-posix" loses
// "arevela" to its alias and is "en-u-va-posix", where go-intl had made the
// keyword first and left "en-posix". Node's answers (ISSUES.md LO-5).
func TestLonePosixAfterAliases(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"en-arevela-posix", "en-u-va-posix"},
		{"en-posix-arevela", "en-u-va-posix"},
		{"hy-arevela-posix", "hy-u-va-posix"},
		{"en-posix", "en-u-va-posix"},
		{"en-US-posix", "en-US-u-va-posix"},
		{"en-posix-heploc", "en-alalc97-posix"},
		{"en-fonipa-posix", "en-fonipa-posix"},
		{"en-x-lvariant-posix", "en-u-va-posix"},
		{"en-arevela-x-lvariant-posix", "en-u-va-posix"},
	} {
		got, err := canon.Canonicalize(c.tag)
		if err != nil || got.String() != c.want {
			t.Errorf("%s: %v, %v; want %s", c.tag, got, err, c.want)
		}
	}
}
