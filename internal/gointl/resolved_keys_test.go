//go:build gointl_all

package intl_test

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// resolvedTypes are the types each service's ResolvedOptions returns.
var resolvedTypes = map[string]reflect.Type{
	"Collator":           reflect.TypeOf(intl.ResolvedCollator{}),
	"DateTimeFormat":     reflect.TypeOf(intl.ResolvedDateTimeFormat{}),
	"DisplayNames":       reflect.TypeOf(intl.ResolvedDisplayNames{}),
	"DurationFormat":     reflect.TypeOf(intl.ResolvedDurationFormat{}),
	"ListFormat":         reflect.TypeOf(intl.ResolvedListFormat{}),
	"NumberFormat":       reflect.TypeOf(intl.ResolvedNumberFormat{}),
	"PluralRules":        reflect.TypeOf(intl.ResolvedPluralRules{}),
	"RelativeTimeFormat": reflect.TypeOf(intl.ResolvedRelativeTimeFormat{}),
	"Segmenter":          reflect.TypeOf(intl.ResolvedSegmenter{}),
}

// durationUnits are DurationFormat's units, whose styles and displays its
// resolved options hold in Units and Display.
var durationUnits = map[string]bool{"years": true, "months": true, "weeks": true, "days": true,
	"hours": true, "minutes": true, "seconds": true, "milliseconds": true, "microseconds": true,
	"nanoseconds": true}

// resolvedField is the Go field that holds a property resolvedOptions
// reports: its name capitalized, but for the few named otherwise.
func resolvedField(service, key string) string {
	switch {
	case service == "DisplayNames" && key == "type":
		return "Kind"
	case service == "DurationFormat" && durationUnits[key]:
		return "Units"
	case service == "DurationFormat" && durationUnits[strings.TrimSuffix(key, "Display")]:
		return "Display"
	}
	r, n := utf8.DecodeRuneInString(key)
	return string(unicode.ToUpper(r)) + key[n:]
}

// TestResolvedOptionsCoverNode holds every service's resolved options to
// having a field for each property Node's resolvedOptions reports, as
// testdata/resolved_keys_node.js finds them, so that none is missing when
// Node reports a new one.
func TestResolvedOptionsCoverNode(t *testing.T) {
	b, err := os.ReadFile("testdata/resolved_keys_node.txt")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		service, keys := fields[0], fields[1:]
		typ, ok := resolvedTypes[service]
		if !ok {
			t.Errorf("%s: no ResolvedOptions", service)
			continue
		}
		seen++
		for _, key := range keys {
			if _, ok := typ.FieldByName(resolvedField(service, key)); !ok {
				t.Errorf("%s: %s has no field for %q", service, typ.Name(), key)
			}
		}
	}
	if seen != len(resolvedTypes) {
		t.Errorf("%d services recorded, want %d", seen, len(resolvedTypes))
	}
}

// hour12 is reported beside the hour cycle: true for h11 and h12. And a
// segmenter reports its locale without the Unicode extension, which it
// reads nothing from. Node's answers.
func TestResolvedHour12AndSegmenter(t *testing.T) {
	for _, c := range []struct {
		opts   intl.DateTimeFormatOptions
		cycle  intl.HourCycle
		hour12 bool
	}{
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric}, intl.H12, true},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H23}, intl.H23, false},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H11}, intl.H11, true},
	} {
		r := newDateTime(t, "en", c.opts).ResolvedOptions()
		if r.HourCycle != c.cycle || r.Hour12 != c.hour12 {
			t.Errorf("%+v: %v, hour12 %v; want %v, %v", c.opts, r.HourCycle, r.Hour12, c.cycle, c.hour12)
		}
	}

	for tag, want := range map[string]string{
		"en-US-u-ca-gregory-nu-arab": "en-US",
		"en-US-u-va-posix":           "en-US-u-va-posix",
		"ja":                         "ja",
	} {
		loc, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		s, err := intl.NewSegmenter(loc, intl.SegmenterOptions{Granularity: intl.GranularityWord})
		if err != nil {
			t.Fatal(err)
		}
		r := s.ResolvedOptions()
		if r.Locale != want || r.Granularity != intl.GranularityWord {
			t.Errorf("%s: %q, %v; want %q, word", tag, r.Locale, r.Granularity, want)
		}
	}
}
