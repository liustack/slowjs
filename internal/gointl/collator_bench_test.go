//go:build gointl_all

package intl

import "testing"

func BenchmarkNewCollator(b *testing.B) {
	l, _ := ParseLocale("de")
	for i := 0; i < b.N; i++ {
		if _, err := NewCollator(l, CollatorOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareASCII(b *testing.B) {
	l, _ := ParseLocale("en")
	c, _ := NewCollator(l, CollatorOptions{})
	for i := 0; i < b.N; i++ {
		c.Compare("Hello, world", "Hello, World")
	}
}

func BenchmarkCompareAccents(b *testing.B) {
	l, _ := ParseLocale("fr")
	c, _ := NewCollator(l, CollatorOptions{})
	for i := 0; i < b.N; i++ {
		c.Compare("résumé côté", "resume cote")
	}
}
