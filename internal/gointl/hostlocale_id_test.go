//go:build gointl_all

package intl

import "testing"

// An ICU locale ID is read as ICU's toLanguageTag writes it: a variant BCP 47
// cannot hold goes into private use after "lvariant", so a POSIX locale with
// a modifier keeps its language, where go-intl had given up and answered
// en-US, and "de@euro" no longer reads as the script Euro (ISSUES.md LO-7).
func TestLocaleFromICUID(t *testing.T) {
	for _, c := range []struct{ id, want string }{
		{"de_DE_euro", "de-DE-x-lvariant-euro"},
		{"de__euro", "de-x-lvariant-euro"},
		{"es_ES_TRADITIONAL", "es-ES"},
		{"ja_JP_TRAD", "ja-JP-x-lvariant-trad"},
		{"en_US_POSIX", "en-US-u-va-posix"},
		{"ca_ES_VALENCIA", "ca-ES-valencia"},
		{"de_DE", "de-DE"},
		{"zh_Hans_CN", "zh-Hans-CN"},
		{"sr_Latn_RS", "sr-Latn-RS"},
		{"es_419", "es-419"},
		{"en", "en"},
	} {
		got, ok := localeFromICUID(c.id)
		if !ok || got.String() != c.want {
			t.Errorf("%s: %v, %v; want %s", c.id, got, ok, c.want)
		}
	}
}
