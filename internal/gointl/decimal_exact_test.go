//go:build gointl_all

package intl_test

import (
	"runtime"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestParseExactDecimal pins that a number too large for a float is written
// in full when read as an exact decimal, as V8 writes a BigInt, and as an
// infinity when read as a string is, as ECMA-402 has it.
func TestParseExactDecimal(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	big := "1" + strings.Repeat("0", 400)
	want := "10" + strings.Repeat(",000", 400/3)
	if got := nf.FormatDecimal(intl.ParseExactDecimal(big)); got != want {
		t.Errorf("exact = %d characters, want %d", len(got), len(want))
	}
	if got := nf.FormatDecimal(intl.ParseExactDecimal("-" + big)); got != "-"+want {
		t.Errorf("negative exact = %.20q", got)
	}
	if got := nf.FormatDecimal(intl.ParseDecimal(big)); got != "∞" {
		t.Errorf("string = %q, want ∞", got)
	}
}

// An exact decimal keeps only digits written out: a string with an exponent
// is read as ParseDecimal reads it, so that "1e1000000000" is an infinity
// rather than a gigabyte of zeros (ISSUES.md API-11). Node's answers for
// the same strings.
func TestParseExactDecimalExponent(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"1e1000000000":  "∞",
		"-1e1000000000": "-∞",
		"1e400":         "∞",
		"-1e400":        "-∞",
		"1.5e10":        "15,000,000,000",
		"0.123e3":       "123",
		"1e-1000000":    "0",
		"1e-400":        "0",
	} {
		if got := nf.FormatDecimal(intl.ParseExactDecimal(in)); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
		if got := nf.FormatDecimal(intl.ParseDecimal(in)); got != want {
			t.Errorf("ParseDecimal %s: %q, want %q", in, got, want)
		}
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	d := intl.ParseExactDecimal("1e1000000000")
	runtime.ReadMemStats(&after)
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("1e1000000000 allocated %d bytes", n)
	}
	if got := nf.FormatDecimal(d); got != "∞" {
		t.Errorf("1e1000000000: %q", got)
	}
}
