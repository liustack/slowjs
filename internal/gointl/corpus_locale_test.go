//go:build gointl_all

package intl_test

import (
	"path/filepath"
	"sort"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

// Every locale the corpus asks for has to parse, and has to parse to itself.
// The corpus is what go-intl will be held against, so a tag it uses that this
// cannot read is not an edge case to get to later: it is coverage that would
// silently never run.
func TestCorpusLocalesParse(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	seen := map[string]bool{}
	for _, c := range f.Cases {
		if c.Locale == "" || seen[c.Locale] {
			continue
		}
		seen[c.Locale] = true

		l, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("line %d: ParseLocale(%q): %v", c.Line, c.Locale, err)
			continue
		}
		// The corpus writes its tags canonically already, so anything else
		// means canonicalization changed a tag ICU did not.
		if got := l.String(); got != c.Locale {
			t.Errorf("ParseLocale(%q) writes itself as %q", c.Locale, got)
		}
	}

	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	if len(tags) != 40 {
		t.Errorf("the corpus uses %d locales, want 40: %v", len(tags), tags)
	}
	t.Logf("%d locales, all parsed: %v", len(tags), tags)
}
