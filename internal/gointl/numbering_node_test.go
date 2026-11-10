//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// numberingGaps are cases these expectations hold go-intl to that it cannot
// meet yet, for a reason that is not about numbering systems. Each is checked
// all the same: one that starts to pass is reported, so the entry goes.
var numberingGaps = map[string]string{}

// TestNumberingSystemsMatchNode holds every numeric numbering system, asked for
// by keyword and by option in locales that do and do not have it, to what
// Node writes. The expectations are written by testdata/numbering_node.js.
func TestNumberingSystemsMatchNode(t *testing.T) {
	f, err := os.Open(filepath.FromSlash("testdata/numbering_node.txt.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(make([]byte, 1<<20), 1<<20)

	var ran, matched, gaps int
	var differences []string
	var (
		lastKey string
		lastNF  *intl.NumberFormat
		lastErr error
	)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c struct {
			Service, Tag string
			Opts         map[string]any
			Value        float64
			Want         string
			Locale, Nu   string
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 7 {
			t.Fatalf("a line that is not seven fields: %s", line)
		}
		for i, dst := range []any{&c.Service, &c.Tag, &c.Opts, &c.Value, &c.Want, &c.Locale, &c.Nu} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		loc, err := intl.ParseLocale(c.Tag)
		if err != nil {
			t.Fatalf("%s: %v", c.Tag, err)
		}
		nu, _ := c.Opts["numberingSystem"].(string)
		name := fmt.Sprintf("%s %s %v %v", c.Service, c.Tag, c.Opts, c.Value)

		var got, gotLocale, gotNu string
		switch c.Service {
		case "NumberFormat":
			// The cases come three values to a formatter, in a row.
			key := string(fields[1]) + string(fields[2])
			nf, err := lastNF, lastErr
			if key != lastKey {
				opts := intl.NumberFormatOptions{NumberingSystem: nu}
				if err := numberOptionsFromMap(c.Opts, &opts); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				nf, err = intl.NewNumberFormat(loc, opts)
				lastKey, lastNF, lastErr = key, nf, err
			}
			if err != nil {
				differences = append(differences, name+": "+err.Error())
				ran++
				continue
			}
			got = nf.Format(c.Value)
			r := nf.ResolvedOptions()
			gotLocale, gotNu = r.Locale, r.NumberingSystem
		case "DateTimeFormat":
			dtf, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
				TimeZone: "UTC", DateStyle: intl.LengthShort, TimeStyle: intl.LengthMedium,
				NumberingSystem: nu, Compat: intl.NodeICU,
			})
			if err != nil {
				differences = append(differences, name+": "+err.Error())
				ran++
				continue
			}
			got = dtf.Format(time.UnixMilli(int64(c.Value)).UTC())
			r := dtf.ResolvedOptions()
			gotLocale, gotNu = r.Locale, r.NumberingSystem
		case "RelativeTimeFormat":
			rtf, err := intl.NewRelativeTimeFormat(loc, intl.RelativeTimeFormatOptions{NumberingSystem: nu})
			if err != nil {
				differences = append(differences, name+": "+err.Error())
				ran++
				continue
			}
			got = rtf.Format(c.Value, intl.RelativeDay)
			r := rtf.ResolvedOptions()
			gotLocale, gotNu = r.Locale, r.NumberingSystem
		default:
			t.Fatalf("a %s case", c.Service)
		}
		ok := got == c.Want && gotLocale == c.Locale && gotNu == c.Nu
		// A gap is about the locale's own calendar, so a case that names
		// another is not in it.
		why, gap := numberingGaps[c.Service+" "+strings.Split(c.Tag, "-u-")[0]]
		if gap && !strings.Contains(c.Tag, "-ca-") {
			if ok {
				differences = append(differences, fmt.Sprintf("%s: passes, so %q is no longer a gap", name, why))
			}
			gaps++
			continue
		}
		ran++
		if ok {
			matched++
			continue
		}
		differences = append(differences, fmt.Sprintf("%s\n   got  %+q %s %s\n   want %+q %s %s",
			name, got, gotLocale, gotNu, c.Want, c.Locale, c.Nu))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly; %d left out as known gaps", matched, ran, gaps)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}

// numberOptionsFromMap reads the few options the numbering cases use.
func numberOptionsFromMap(in map[string]any, out *intl.NumberFormatOptions) error {
	for key, value := range in {
		switch key {
		case "numberingSystem":
		case "style":
			switch value {
			case "percent":
				out.Style = intl.StylePercent
			case "currency":
				out.Style = intl.StyleCurrency
			default:
				return fmt.Errorf("style %v", value)
			}
		case "currency":
			out.Currency, _ = value.(string)
		case "notation":
			switch value {
			case "scientific":
				out.Notation = intl.NotationScientific
			case "compact":
				out.Notation = intl.NotationCompact
			default:
				return fmt.Errorf("notation %v", value)
			}
		default:
			return fmt.Errorf("option %q", key)
		}
	}
	return nil
}
