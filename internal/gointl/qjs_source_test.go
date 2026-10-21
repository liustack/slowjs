// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// A qjsProbe uses one service the way a program does, from a source, and
// says what it wrote.
type qjsProbe struct {
	name string
	run  func(src Source) (string, error)
}

func qjsLocale(s string) Locale {
	l, err := ParseLocale(s)
	if err != nil {
		panic(err)
	}
	return l
}

var qjsMoment = time.Date(2024, 3, 5, 14, 4, 5, 0, time.UTC)

var qjsProbes = []qjsProbe{
	{"NumberFormat", func(src Source) (string, error) {
		f, err := NewNumberFormatFrom(src, qjsLocale("de"), NumberFormatOptions{Style: StyleCurrency, Currency: "EUR"})
		if err != nil {
			return "", err
		}
		g, err := NewNumberFormatFrom(src, qjsLocale("zh-CN"), NumberFormatOptions{Notation: NotationCompact})
		if err != nil {
			return "", err
		}
		return f.Format(1234567.891) + " " + g.Format(123456789), nil
	}},
	{"DateTimeFormat", func(src Source) (string, error) {
		f, err := NewDateTimeFormatFrom(src, qjsLocale("ja-JP"), DateTimeFormatOptions{
			TimeZone: "Asia/Tokyo", DateStyle: LengthFull, TimeStyle: LengthLong})
		if err != nil {
			return "", err
		}
		return f.Format(qjsMoment), nil
	}},
	{"DateTimeFormat zone name", func(src Source) (string, error) {
		f, err := NewDateTimeFormatFrom(src, qjsLocale("fr"), DateTimeFormatOptions{
			TimeZone: "America/Chicago", TimeZoneName: ZoneLong, Hour: WidthNumeric})
		if err != nil {
			return "", err
		}
		return f.Format(qjsMoment), nil
	}},
	{"PluralRules", func(src Source) (string, error) {
		var out []string
		for _, l := range []string{"ar", "cy", "ru"} {
			p, err := NewPluralRulesFrom(src, qjsLocale(l), PluralRulesOptions{})
			if err != nil {
				return "", err
			}
			out = append(out, fmt.Sprintf("%v/%v", p.Select(2), p.Select(3)))
		}
		return strings.Join(out, " "), nil
	}},
	{"Collator", func(src Source) (string, error) {
		var out []string
		for _, c := range [][3]string{{"de", "Äpfel", "Birnen"}, {"sv", "ö", "z"}, {"zh", "中", "国"}} {
			col, err := NewCollatorFrom(src, qjsLocale(c[0]), CollatorOptions{})
			if err != nil {
				return "", err
			}
			out = append(out, fmt.Sprint(col.Compare(c[1], c[2])))
		}
		return strings.Join(out, " "), nil
	}},
	{"Segmenter", func(src Source) (string, error) {
		var out []string
		for _, c := range [][2]string{{"ja", "今日は良い天気です"}, {"zh", "我们在北京工作"}, {"th", "สวัสดีครับ"}} {
			s, err := NewSegmenterFrom(src, qjsLocale(c[0]), SegmenterOptions{Granularity: GranularityWord})
			if err != nil {
				return "", err
			}
			text := utf16.Encode([]rune(c[1]))
			var words []string
			for _, seg := range s.Segment(text).All() {
				words = append(words, string(utf16.Decode(text[seg.Index:seg.End])))
			}
			out = append(out, strings.Join(words, "|"))
		}
		return strings.Join(out, " "), nil
	}},
	{"ListFormat", func(src Source) (string, error) {
		f, err := NewListFormatFrom(src, qjsLocale("es"), ListFormatOptions{Type: Conjunction})
		if err != nil {
			return "", err
		}
		return f.Format([]string{"uno", "dos", "tres"}), nil
	}},
	{"RelativeTimeFormat", func(src Source) (string, error) {
		f, err := NewRelativeTimeFormatFrom(src, qjsLocale("ko"), RelativeTimeFormatOptions{Numeric: RelativeAuto})
		if err != nil {
			return "", err
		}
		return f.Format(-1, RelativeDay) + " " + f.Format(3, RelativeMonth), nil
	}},
	{"DisplayNames", func(src Source) (string, error) {
		var out []string
		for _, kind := range []DisplayKind{DisplayRegion, DisplayLanguage, DisplayCurrency} {
			d, err := NewDisplayNamesFrom(src, qjsLocale("zh"), DisplayNamesOptions{Kind: kind})
			if err != nil {
				return "", err
			}
			code := map[DisplayKind]string{DisplayRegion: "US", DisplayLanguage: "fr-CA", DisplayCurrency: "JPY"}[kind]
			name, _ := d.Of(code)
			out = append(out, name)
		}
		return strings.Join(out, " "), nil
	}},
	{"DurationFormat", func(src Source) (string, error) {
		f, err := NewDurationFormatFrom(src, qjsLocale("en"), DurationFormatOptions{Style: DurationLong})
		if err != nil {
			return "", err
		}
		var d Duration
		d[DurationHours], d[DurationMinutes] = 2, 5
		return f.Format(d)
	}},
}

// qjsExpected is what the probes write from go-intl's own data.
var qjsExpected = map[string]string{
	"NumberFormat":             "1.234.567,89\u00a0€ 1.2亿",
	"DateTimeFormat":           "2024年3月5日火曜日 23:04:05 JST",
	"DateTimeFormat zone name": "08 h heure normale du centre nord-américain",
	"PluralRules":              "two/few two/few few/few",
	"Collator":                 "-1 1 1",
	"Segmenter":                "今日|は|良い|天気|です 我们|在|北京|工作 สวัสดี|ครับ",
	"ListFormat":               "uno, dos y tres",
	"RelativeTimeFormat":       "어제 3개월 후",
	"DisplayNames":             "美国 加拿大法语 日元",
	"DurationFormat":           "2 hours, 5 minutes",
}

// qjsLeftOut says whether a probe uses data a build tag leaves out, which
// qjs_tags_test.go checks instead.
func qjsLeftOut(probe string) bool {
	return probe == "DisplayNames" && qjsNoNames || probe == "Segmenter" && qjsNoCJDict
}

func TestQJSProbes(t *testing.T) {
	for _, p := range qjsProbes {
		if qjsLeftOut(p.name) {
			continue
		}
		got, err := p.run(Embedded)
		if err != nil {
			t.Errorf("%s: %v", p.name, err)
			continue
		}
		if want, ok := qjsExpected[p.name]; !ok || got != want {
			t.Errorf("%s: %q, want %q", p.name, got, want)
		}
	}
}

// With QJS_GOINTL_DATA naming go-intl's data directory, every probe writes
// from our packs what it writes from go-intl's data.
func TestQJSProbesMatchUpstream(t *testing.T) {
	dir := os.Getenv("QJS_GOINTL_DATA")
	if dir == "" {
		t.Skip("QJS_GOINTL_DATA is not set")
	}
	upstream := NewFS(os.DirFS(dir))
	for _, p := range qjsProbes {
		if qjsLeftOut(p.name) {
			continue
		}
		want, err := p.run(upstream)
		if err != nil {
			t.Fatalf("%s from go-intl's data: %v", p.name, err)
		}
		got, err := p.run(Embedded)
		if err != nil || got != want {
			t.Errorf("%s: %q, %v; go-intl's data: %q", p.name, got, err, want)
		}
	}
}

// Chromium's languages are kept and others are not: Maori and Tatar have
// no data, Welsh has its plural rules only.
func TestQJSAvailable(t *testing.T) {
	for _, c := range []struct {
		service Service
		tag     string
		want    bool
	}{
		{ServiceNumberFormat, "de-AT", true},
		{ServiceNumberFormat, "zh-Hant-HK", true},
		{ServiceDateTimeFormat, "ja", true},
		{ServiceCollator, "zh", true},
		{ServiceNumberFormat, "mi", false},
		{ServiceDateTimeFormat, "tt", false},
		{ServicePluralRules, "cy", true},
		{ServiceNumberFormat, "cy", false},
	} {
		m, err := NewLocaleMatcher(Embedded, c.service)
		if err != nil {
			t.Fatal(err)
		}
		if got := m.Available(c.tag); got != c.want {
			t.Errorf("%s available in %s: %v, want %v", c.service, c.tag, got, c.want)
		}
	}
}

// Every chunk of every pack inflates, and every file reads.
func TestQJSPacksRead(t *testing.T) {
	packs, err := qjsOpenPacks(qjsPackData())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range packs {
		for _, name := range p.Names() {
			if _, err := p.File(name); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

// A formatter inflates only the chunks it reads: numbers leave the display
// names and the dictionaries alone, and read the pool of number strings in
// place, which inflated is 2 MB.
func TestQJSInflatesOnlyWhatIsRead(t *testing.T) {
	packs, err := qjsOpenPacks(qjsPackData())
	if err != nil {
		t.Fatal(err)
	}
	src := qjsSource{qjsFiles{packs}}
	if _, err := NewNumberFormatFrom(src, qjsLocale("de"), NumberFormatOptions{}); err != nil {
		t.Fatal(err)
	}
	total := 0
	built := qjsPackData()
	for i, p := range packs {
		n, size := p.Inflated()
		if built[i].name != "core" && n != 0 {
			t.Errorf("%s: %d chunks inflated", built[i].name, n)
		}
		total += size
	}
	if total == 0 || total > 1<<20 {
		t.Errorf("a NumberFormat inflated %d bytes", total)
	}
}
