//go:build gointl_all

package colldata

import "testing"

// TestBuildTrie holds a built trie to the values it was built from, for
// values that vary in the Basic Multilingual Plane, the supplementary
// planes and not at all.
func TestBuildTrie(t *testing.T) {
	for name, get := range map[string]func(rune) uint32{
		"constant": func(rune) uint32 { return 0xc0 },
		"bmp": func(c rune) uint32 {
			if c >= 0x1100 && c < 0x1200 {
				return uint32(c) << 8
			}
			return 0xc0
		},
		"supplementary": func(c rune) uint32 {
			switch {
			case c%7 == 0 && c >= 0x1d000 && c < 0x1f000:
				return uint32(c)
			case c >= 0xe0000 && c < 0xe0080:
				return 5
			}
			return 0xc0
		},
		"high": func(c rune) uint32 {
			if c > 0x10fff0 {
				return 1
			}
			return 2
		},
	} {
		tr, err := BuildTrie(get, 0xffffffff)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for c := rune(0); c <= 0x10ffff; c++ {
			if got, want := tr.Get(c), get(c); got != want {
				t.Fatalf("%s: U+%04X is %#x, want %#x", name, c, got, want)
			}
		}
		if got := tr.Get(0x110000); got != 0xffffffff {
			t.Errorf("%s: past the code points is %#x, want the error value", name, got)
		}
	}
}
