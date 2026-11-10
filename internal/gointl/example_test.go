//go:build gointl_all

package intl_test

import (
	"fmt"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The README's example. The no-break spaces are CLDR's, and %q shows them.
func Example() {
	loc, err := intl.ParseLocale("de-DE")
	if err != nil {
		panic(err)
	}

	nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "EUR",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", nf.Format(1234.5))
	r, _ := nf.FormatRange(5, 10)
	fmt.Printf("%q\n", r)

	df, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
		TimeZone: "Europe/Berlin", DateStyle: intl.LengthLong, TimeStyle: intl.LengthShort,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", df.Format(time.Date(2024, 1, 5, 14, 4, 5, 0, time.UTC)))

	pr, err := intl.NewPluralRules(loc, intl.PluralRulesOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println(pr.Select(1), pr.Select(2))

	col, err := intl.NewCollator(loc, intl.CollatorOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println(col.Compare("Äpfel", "Birnen"))
	// Output:
	// "1.234,50\u00a0€"
	// "5,00–10,00\u00a0€"
	// "5. Januar 2024 um 15:04"
	// one other
	// -1
}

// A formatter writes in the locale it is given, as it is. ECMA-402 first
// resolves the requested locale among the service's available locales,
// which is what makes Node format "az-Arab" in Azerbaijani's Latin data:
// ICU has no number data of its own for it. LocaleMatcher is that step.
func ExampleLocaleMatcher_Resolve() {
	requested, err := intl.ParseLocale("az-Arab")
	if err != nil {
		panic(err)
	}
	m, err := intl.NewLocaleMatcher(intl.Embedded, intl.ServiceNumberFormat)
	if err != nil {
		panic(err)
	}
	def, _ := intl.ParseLocale("en-US")
	resolved := m.Resolve([]intl.Locale{requested}, intl.BestFit, def)
	f, err := intl.NewNumberFormat(resolved, intl.NumberFormatOptions{Notation: intl.NotationCompact})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s %q\n", resolved, f.Format(1234567))
	// Output:
	// az "1,2\u00a0mln"
}
