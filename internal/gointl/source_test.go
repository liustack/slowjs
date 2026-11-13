//go:build gointl_all

package intl_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
)

func mustLocale(t *testing.T, s string) intl.DataLocale {
	t.Helper()
	l, err := intl.ParseLocale(s)
	if err != nil {
		t.Fatalf("ParseLocale(%q): %v", s, err)
	}
	return l.Data()
}

// The generator writes what the reader reads. The two share one definition of
// the layout, and this is what proves they have not drifted: the tables on
// disk are searched for entries whose values are known independently, from
// CLDR's own files.
func TestEmbeddedTablesRoundTrip(t *testing.T) {
	f, err := intl.NewFallbacker(intl.Embedded)
	if err != nil {
		t.Fatalf("building a fallbacker from the embedded data: %v", err)
	}

	// Likely subtags, as CLDR's likelySubtags.json gives them.
	for _, c := range []struct{ in, want string }{
		{"zh", "zh-Hans-CN"},
		{"en", "en-Latn-US"},
		{"de", "de-Latn-DE"},
		{"und", "en-Latn-US"},
		{"ja", "ja-Jpan-JP"},
		// A script or region already given is kept rather than overruled.
		{"zh-TW", "zh-Hant-TW"},
		{"zh-Hant", "zh-Hant-TW"},
		{"und-Hant", "zh-Hant-TW"},
		{"und-TW", "zh-Hant-TW"},
	} {
		got, ok := f.Maximize(mustLocale(t, c.in))
		if !ok {
			t.Errorf("Maximize(%q): nothing found", c.in)
			continue
		}
		if got.String() != c.want {
			t.Errorf("Maximize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The reason the parent table exists: truncation alone sends these to the
// wrong place, and a wrong fallback is a wrong answer rather than a missing
// one, which is the harder kind to notice.
func TestParentLocalesRedirectTheChain(t *testing.T) {
	f, err := intl.NewFallbacker(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		want []string
	}{
		// Traditional Chinese must not inherit from simplified.
		{"zh-Hant", []string{"zh-Hant", "und"}},
		// The English of the world outside America is shared.
		{"en-AU", []string{"en-AU", "en-001", "en", "und"}},
		{"en-GB", []string{"en-GB", "en-001", "en", "und"}},
		// Latin American Spanish likewise.
		{"es-AR", []string{"es-AR", "es-419", "es", "und"}},
		// Where CLDR says nothing, truncation is still what happens.
		{"de-CH", []string{"de-CH", "de", "und"}},
		{"en", []string{"en", "und"}},
		{"und", []string{"und"}},
	}
	for _, c := range cases {
		var got []string
		for _, d := range f.Chain(mustLocale(t, c.in)) {
			got = append(got, d.String())
		}
		if !equalStrings(got, c.want) {
			t.Errorf("Chain(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// Truncation on its own gives a different answer for these, which is the whole
// reason stage 2 exists. If this ever stops differing, the data is not loaded.
func TestDataChangesTheChain(t *testing.T) {
	f, err := intl.NewFallbacker(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"zh-Hant", "en-AU", "es-AR"} {
		d := mustLocale(t, s)
		plain, informed := d.Fallback(), f.Chain(d)
		if equalLocales(plain, informed) {
			t.Errorf("%q chains the same way with and without CLDR's data: %v",
				s, informed)
		}
	}
}

func TestMinimize(t *testing.T) {
	f, err := intl.NewFallbacker(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	// UTS #35 tries the language alone, then the language with its region,
	// then the language with its script, and takes the first whose maximized
	// form is the one it started from. So traditional Chinese in Taiwan comes
	// back as "zh-TW" rather than "zh-Hant", which is what ICU answers too.
	for _, c := range []struct{ in, want string }{
		{"zh-Hans-CN", "zh"},
		{"en-Latn-US", "en"},
		{"zh-Hant-TW", "zh-TW"},
		{"zh-TW", "zh-TW"},
		{"de-Latn-DE", "de"},
	} {
		got, ok := f.Minimize(mustLocale(t, c.in))
		if !ok {
			t.Errorf("Minimize(%q): nothing found", c.in)
			continue
		}
		if got.String() != c.want {
			t.Errorf("Minimize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A source is an interface so that the data can come from somewhere else. The
// embedded tables are one backing of the same reader; a directory is another,
// and this is what says so rather than the documentation saying it.
func TestSourceCanBeReplaced(t *testing.T) {
	// Build a table by hand: one parent, pointing somewhere CLDR does not.
	from, to := mustLocale(t, "xx-YY"), mustLocale(t, "zz")
	rec := []byte{layout.Pairs}
	rec, _ = from.AppendBinary(rec)
	rec, _ = to.AppendBinary(rec)

	// Likely subtags may be empty, but for the table's version; a source
	// only has to answer.
	fsys := fstest.MapFS{
		"parentlocales.bin": {Data: rec},
		"likelysubtags.bin": {Data: []byte{layout.Pairs}},
	}
	f, err := intl.NewFallbacker(intl.NewFS(fsys))
	if err != nil {
		t.Fatalf("building a fallbacker from a made-up source: %v", err)
	}
	got := f.Chain(from)
	want := []string{"xx-YY", "zz", "und"}
	var names []string
	for _, d := range got {
		names = append(names, d.String())
	}
	if !equalStrings(names, want) {
		t.Errorf("Chain = %v, want %v", names, want)
	}
}

// The embedded data is a default, not a requirement, so a source that has
// nothing says so rather than the package failing to start.
func TestMissingDataIsReported(t *testing.T) {
	_, err := intl.NewFallbacker(intl.NewFS(fstest.MapFS{}))
	if err == nil {
		t.Fatal("an empty source built a fallbacker")
	}
	if !errors.Is(err, intl.ErrNotFound) {
		t.Errorf("an empty source gave %v, which is not ErrNotFound", err)
	}
}

// A table whose length is not a whole number of records is refused rather than
// read as though the last record were complete.
func TestTruncatedTableIsRefused(t *testing.T) {
	fsys := fstest.MapFS{
		"parentlocales.bin": {Data: append([]byte{layout.Pairs}, make([]byte, 2*intl.DataLocaleSize-1)...)},
		"likelysubtags.bin": {Data: []byte{layout.Pairs}},
	}
	if _, err := intl.NewFallbacker(intl.NewFS(fsys)); err == nil {
		t.Fatal("a table cut short was accepted")
	}
}

// The tables on disk are what the generator last wrote: the version, then
// records. A record count that does not divide evenly means the file was
// edited or truncated.
func TestGeneratedTablesAreWellFormed(t *testing.T) {
	for _, name := range []string{"likelysubtags.bin", "parentlocales.bin"} {
		b, err := os.ReadFile(filepath.Join("data", name))
		if err != nil {
			t.Errorf("reading %s: %v", name, err)
			continue
		}
		if len(b) <= 1 {
			t.Errorf("%s is empty; run go run ./internal/localegen", name)
			continue
		}
		if b[0] != layout.Pairs {
			t.Errorf("%s is of version %d, want %d", name, b[0], layout.Pairs)
		}
		size := 2 * intl.DataLocaleSize
		if (len(b)-1)%size != 0 {
			t.Errorf("%s is %d bytes after its version, which is not a whole number of %d-byte records",
				name, len(b)-1, size)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalLocales(a, b []intl.DataLocale) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
