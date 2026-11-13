//go:build gointl_all

package intl

import "testing"

// The decomposition tables are keyed by the character itself, and a
// character outside the range a table holds is answered without searching
// it. The ends of that range are its own records, which must still be
// found: the first and last character each table decomposes, and the
// characters either side of them, which it does not.
func TestDecompositionRangeEnds(t *testing.T) {
	n, err := NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		r    rune
		to   string
		have bool
	}{
		{0x00C0, "À", true},          // the first canonical decomposition, À
		{0x2FA1D, "\U0002A600", true}, // the last, a CJK compatibility ideograph
		{0x00BF, "", false},           // just before the first, ¿
		{0x2FA1E, "", false},          // just after the last
	} {
		if to, ok := n.tables.Decompose(c.r); ok != c.have || to != c.to {
			t.Errorf("%U decomposes to %q, %v, want %q, %v", c.r, to, ok, c.to, c.have)
		}
	}
	// The compatibility table begins earlier, at the no-break space, which
	// has no canonical decomposition at all.
	if to, ok := n.tables.DecomposeCompatibility(0x00A0); !ok || to != " " {
		t.Errorf("U+00A0 decomposes to %q, %v, want a space", to, ok)
	}
	if _, ok := n.tables.Decompose(0x00A0); ok {
		t.Error("U+00A0 has a canonical decomposition")
	}
}

// Looking a character up reads the table where it lies: the key is
// compared as the number it was written from rather than built back into a
// string, and a character that decomposes to nothing -- which text is
// mostly made of -- allocates nothing at all (DESIGN.md, "Read where it
// lies").
func TestDecompositionLookupAllocatesNothing(t *testing.T) {
	n, err := NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	const text = "The quick brown fox. 日本語の文章。中文句子。" // nothing here decomposes
	var found bool
	allocs := testing.AllocsPerRun(100, func() {
		for _, r := range text {
			if _, ok := n.tables.Decompose(r); ok {
				found = true
			}
			if _, ok := n.tables.DecomposeCompatibility(r); ok {
				found = true
			}
			n.tables.Class(r)
		}
	})
	if found {
		t.Fatal("a character in the text decomposes: the test reads the wrong path")
	}
	if allocs != 0 {
		t.Errorf("looking up %d characters made %v allocations", len([]rune(text)), allocs)
	}
}
