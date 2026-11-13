//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

var durationUnitNames = [intl.DurationUnits]string{"years", "months", "weeks", "days", "hours",
	"minutes", "seconds", "milliseconds", "microseconds", "nanoseconds"}

var durationUnitStyles = map[string]intl.DurationUnitStyle{
	"long": intl.DurationUnitLong, "short": intl.DurationUnitShort, "narrow": intl.DurationUnitNarrow,
	"numeric": intl.DurationUnitNumeric, "2-digit": intl.DurationUnitTwoDigit,
}

var durationStyles = map[string]intl.DurationStyle{
	"short": intl.DurationShort, "long": intl.DurationLong, "narrow": intl.DurationNarrow,
	"digital": intl.DurationDigital,
}

var durationDisplays = map[string]intl.DurationDisplay{
	"auto": intl.DurationDisplayAuto, "always": intl.DurationDisplayAlways,
}

// durationOptions reads a JavaScript option bag.
func durationOptions(t *testing.T, raw map[string]any) intl.DurationFormatOptions {
	t.Helper()
	// Node's expectations hold Node's arithmetic, overflow and all.
	o := intl.DurationFormatOptions{Compat: intl.NodeICU}
	for key, v := range raw {
		switch key {
		case "style":
			o.Style = durationStyles[v.(string)]
		case "numberingSystem":
			o.NumberingSystem = v.(string)
		case "fractionalDigits":
			n := int(v.(float64))
			o.FractionalDigits = &n
		default:
			found := false
			for i, name := range durationUnitNames {
				switch key {
				case name:
					o.Units[i], found = durationUnitStyles[v.(string)], true
				case name + "Display":
					o.Display[i], found = durationDisplays[v.(string)], true
				}
			}
			if !found {
				t.Fatalf("an option go-intl has no field for: %s", key)
			}
		}
	}
	return o
}

// resolvedDuration writes resolved options as Node's resolvedOptions does.
func resolvedDuration(r intl.ResolvedDurationFormat) map[string]any {
	out := map[string]any{"locale": r.Locale, "numberingSystem": r.NumberingSystem}
	for name, s := range durationStyles {
		if s == r.Style {
			out["style"] = name
		}
	}
	for i, unit := range durationUnitNames {
		for name, s := range durationUnitStyles {
			if s == r.Units[i] {
				out[unit] = name
			}
		}
		for name, d := range durationDisplays {
			if d == r.Display[i] {
				out[unit+"Display"] = name
			}
		}
	}
	if r.FractionalDigits != nil {
		out["fractionalDigits"] = float64(*r.FractionalDigits)
	}
	return out
}

// durationGaps are durations Node writes differently for a reason go-intl
// does not copy, each with the reason.
var durationGaps = []struct{ duration, why string }{}

// durationLocaleGaps are locales Node writes otherwise, for reasons that
// belong to the data rather than to DurationFormat.
var durationLocaleGaps = map[string]string{}

// TestDurationFormatMatchesNode replays testdata/duration_node.js.
func TestDurationFormatMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/duration_node.txt.gz")
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
	// Each locale is resolved among DurationFormat's available locales, as a
	// host does before building the formatter, with Node's default.
	matcher, err := intl.NewLocaleMatcher(intl.Embedded, intl.ServiceDurationFormat)
	if err != nil {
		t.Fatal(err)
	}
	def, _ := intl.ParseLocale("en-US")
	// The lines come grouped by formatter, so only the last is kept.
	type key struct{ locale, options string }
	var last key
	var df *intl.DurationFormat
	var dfErr error
	var ran, matched, gaps int
	gapPasses, gapFails := map[string]int{}, map[string]int{}
	localeFails := map[string]int{}
	var diffs []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var locale string
		var raw map[string]any
		json.Unmarshal(c[0], &locale)
		json.Unmarshal(c[1], &raw)
		if k := (key{locale, string(c[1])}); k != last {
			loc, err := intl.ParseLocale(locale)
			if err != nil {
				t.Fatal(err)
			}
			loc = matcher.Resolve([]intl.Locale{loc}, intl.BestFit, def)
			df, dfErr = intl.NewDurationFormat(loc, durationOptions(t, raw))
			last = k
		}
		ran++
		if string(c[2]) == "null" {
			// Node refused the options.
			if dfErr == nil {
				diffs = append(diffs, fmt.Sprintf("%s %s: accepted, Node throws", locale, c[1]))
			} else {
				matched++
			}
			continue
		}
		if dfErr != nil {
			diffs = append(diffs, fmt.Sprintf("%s %s: %v", locale, c[1], dfErr))
			continue
		}
		fail := func(text string) {
			if _, gap := durationLocaleGaps[locale]; gap {
				localeFails[locale]++
				gaps++
				ran--
				return
			}
			diffs = append(diffs, text)
		}
		var want map[string]any
		json.Unmarshal(c[4], &want)
		got := resolvedDuration(df.ResolvedOptions())
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if string(gotJSON) != string(wantJSON) {
			fail(fmt.Sprintf("%s %s resolved:\n\t\tgot  %s\n\t\twant %s", locale, c[1], gotJSON, wantJSON))
			continue
		}

		// An empty duration is refused by the JavaScript that reads it, a
		// TypeError, before any formatting.
		if string(c[2]) == "{}" {
			ran--
			continue
		}
		var amounts map[string]float64
		json.Unmarshal(c[2], &amounts)
		var d intl.Duration
		for i, name := range durationUnitNames {
			d[i] = amounts[name]
		}
		parts, err := df.FormatToParts(d)
		gotParts := [][3]string{}
		for _, p := range parts {
			gotParts = append(gotParts, [3]string{string(p.Kind), p.Value, p.Unit})
		}
		gotText, _ := json.Marshal(gotParts)
		if err != nil {
			gotText = []byte("error: " + err.Error())
		}
		var wantParts [][3]string
		json.Unmarshal(c[3], &wantParts)
		wantText, _ := json.Marshal(wantParts)
		if string(c[3]) == "null" {
			wantText = []byte("an error")
		}
		same := string(gotText) == string(wantText) || err != nil && string(c[3]) == "null"
		gap := false
		for _, g := range durationGaps {
			if g.duration == string(c[2]) {
				gap = true
				if same {
					gapPasses[g.duration]++
				} else {
					gapFails[g.duration]++
				}
			}
		}
		if gap {
			gaps++
			ran--
			continue
		}
		if same {
			matched++
			continue
		}
		fail(fmt.Sprintf("%s %s %s:\n\t\tgot  %s\n\t\twant %s", locale, c[1], c[2], gotText, wantText))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node; %d left out as known gaps", matched, ran, gaps)
	for tag := range durationLocaleGaps {
		if localeFails[tag] == 0 {
			t.Errorf("the locale gap %s no longer differs; remove it", tag)
		}
	}
	for d, n := range gapPasses {
		if gapFails[d] == 0 {
			t.Errorf("the gap %s passes in all %d cases; remove it", d, n)
		}
	}
	if len(diffs) > 0 {
		if len(diffs) > 25 {
			diffs = diffs[:25]
		}
		t.Errorf("%d differ, the first:\n\t%s", ran-matched, strings.Join(diffs, "\n\t"))
	}
}
