//go:build gointl_all

package eastasian

import "testing"

// ParseTables refuses a year no table can hold, which a Source other than
// the embedded data can hand it: month 13 or 00 had panicked, indexing past
// GregorianFixed's table, and a leap month's ordinal of 99 been taken
// (ISSUES.md API-8).
func TestParseTablesRefusesMalformedYears(t *testing.T) {
	for _, line := range []string{
		"china 1912 lslsslssllsls 0 1912-13-18",
		"china 1912 lslsslssllsls 0 1912-00-18",
		"china 1912 lslsslssllsls 0 1912-02-00",
		"china 1912 lslsslssllsls 0 1912-01-32",
		"china 1913 lslsslssllsls 0 1913-02-29",
		"china 1900 lslsslssllsls 0 1900-02-29",
		"china 1912 lslsslssllsls 99 1912-02-18",
		"china 1912 lslsslssllsls 1 1912-02-18",
		"china 1912 lslsslssllsls -1 1912-02-18",
		"china 1912 lslsslssllsl 6 1912-02-18",
		"china 1912 lslsslssll 0 1912-02-18",
		"china 1912 lslsslssllslsl 0 1912-02-18",
	} {
		if tables, err := ParseTables([]byte(line+"\n"), "china"); err == nil {
			t.Errorf("%q: %v, want an error", line, tables["china"])
		}
	}
	for _, c := range []struct {
		line string
		want TableYear
	}{
		{"china 1914 llslslslsslsl 6 1914-01-26",
			TableYear{Year: 1914, Leap: 6, Count: 13, Long: 0b1010010101011, Start: GregorianFixed(1914, 1, 26)}},
		{"china 1912 lslsslssllsls 13 2000-02-29",
			TableYear{Year: 1912, Leap: 13, Count: 13, Long: 0b0101100100101, Start: GregorianFixed(2000, 2, 29)}},
		{"ummalqura 1300 lslslslslsls 0 1882-11-12",
			TableYear{Year: 1300, Count: 12, Long: 0b010101010101, Start: GregorianFixed(1882, 11, 12)}},
	} {
		tables, err := ParseTables([]byte(c.line + "\n"))
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		for _, years := range tables {
			if len(years) != 1 || years[0] != c.want {
				t.Errorf("%q: %+v, want %+v", c.line, years, c.want)
			}
		}
	}
}
