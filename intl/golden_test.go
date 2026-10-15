// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// The golden test compares this package with a browser. testdata/chromium
// holds what Chromium answered for a few thousand expressions, recorded by
// record.cjs; go test only reads it. To record again, after changing the
// corpus or for a new browser:
//
//	go test ./intl -run TestGolden -args -golden.corpus   # writes corpus.json
//	node intl/testdata/chromium/record.cjs golden   # writes golden.json.gz

var (
	goldenCorpus = flag.Bool("golden.corpus", false, "write testdata/chromium/corpus.json and skip the comparison")
	goldenRun    = flag.String("golden.run", "", "compare only the expressions matching this regular expression")
	goldenShow   = flag.Int("golden.show", 40, "report at most this many unexplained differences")
)

var goldenDir = filepath.Join("testdata", "chromium")

var goldenCalendar = regexp.MustCompile(`-u-ca-(\w+)`)

// goldenDifference is a difference from Chromium that is understood: the
// expression, Chromium's answer and ours match a known cause. Anything else
// fails the test.
type goldenDifference struct {
	why   string
	match func(expr, got, want string) bool
}

var goldenDifferences = []goldenDifference{
	{
		// CLDR 48.2 changed the grouping separator of fr-CH from U+202F to an
		// apostrophe. Chromium 156 is on CLDR 48.1.
		why: "fr-CH groups with ' in CLDR 48.2, Chromium has 48.1",
		match: func(expr, got, want string) bool {
			return strings.Contains(expr, `"fr-CH"`) && strings.ReplaceAll(got, "'", "\u202f") == want
		},
	},
	{
		// ICU takes the symbols of a numbering system a locale has no data
		// for from CLDR root, which defines them for arab. cldr-json publishes
		// root's latn symbols only, so the latn ones are used here.
		why: "arab digits in a locale without arab symbols: root's symbols are not in cldr-json",
		match: func(expr, got, want string) bool {
			return strings.Contains(expr, "numberingSystem:'arab'") && !strings.Contains(expr, `"ar`)
		},
	},
	{
		// Only the Gregorian calendar has data everywhere; buddhist and
		// persian have it where they are the default (th, fa). Any other
		// calendar is dropped from the locale and formats as Gregorian.
		why: "a calendar there is no data for formats as Gregorian",
		match: func(expr, got, want string) bool {
			m := goldenCalendar.FindStringSubmatch(expr)
			return m != nil && m[1] != "gregory"
		},
	},
	{
		// CLDR has 미 (America) where Chromium's ICU data has 북미 (North
		// America), and 미 산지 where it has 북미 로키 산맥 (Rocky Mountain).
		why: "ko names the North American zones 미, Chromium has 북미",
		match: func(expr, got, want string) bool {
			return strings.Contains(expr, `"ko"`) && strings.Contains(want, "북미") && !strings.Contains(got, "북미")
		},
	},
}

type golden struct {
	Browser   string      `json:"browser"`
	UserAgent string      `json:"userAgent"`
	Cases     [][2]string `json:"cases"`
}

func readGolden(tb testing.TB) *golden {
	f, err := os.Open(filepath.Join(goldenDir, "golden.json.gz"))
	if err != nil {
		tb.Fatal(err)
	}

	defer f.Close()

	z, err := gzip.NewReader(f)
	if err != nil {
		tb.Fatal(err)
	}

	var g golden
	if err := json.NewDecoder(z).Decode(&g); err != nil {
		tb.Fatal(err)
	}

	return &g
}

func TestGolden(t *testing.T) {
	if *goldenCorpus {
		b, err := json.MarshalIndent(goldenExprs(), "", "\t")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(goldenDir, "corpus.json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}

		t.Skip("wrote corpus.json")
	}

	g := readGolden(t)
	if *goldenRun != "" {
		re := regexp.MustCompile(*goldenRun)
		cases := g.Cases[:0]
		for _, c := range g.Cases {
			if re.MatchString(c[0]) {
				cases = append(cases, c)
			}
		}
		g.Cases = cases
	}
	vm := newVM(t)
	byCause := map[string]int{}
	var unexplained []string
	for _, c := range g.Cases {
		expr, want := c[0], c[1]
		got := goldenEval(t, vm, expr)
		if got == want {
			continue
		}

		why := ""
		for _, d := range goldenDifferences {
			if d.match(expr, got, want) {
				why = d.why
				break
			}
		}
		byCause[why]++
		if why == "" {
			unexplained = append(unexplained, fmt.Sprintf("%s\n\tgot    %q\n\tbrowser %q", expr, got, want))
		}
	}
	var causes []string
	known := 0
	for why, n := range byCause {
		if why != "" {
			known += n
			causes = append(causes, fmt.Sprintf("%5d %s", n, why))
		}
	}
	sort.Strings(causes)
	for _, c := range causes {
		t.Log(c)
	}
	for i, u := range unexplained {
		if i == *goldenShow {
			t.Errorf("... and %d more", len(unexplained)-i)
			break
		}

		t.Error(u)
	}
	t.Logf("%s: %d of %d expressions match exactly, %d differ for a known reason, %d are unexplained",
		g.Browser, len(g.Cases)-known-len(unexplained), len(g.Cases), known, len(unexplained))
}

// goldenEval evaluates expr, returning an error's type rather than failing:
// the two implementations have to agree on what throws, not on the wording.
func goldenEval(t *testing.T, vm *quickjs.VM, expr string) string {
	t.Helper()
	v, err := vm.Eval("(() => {try {return String("+expr+")} catch (e) {return 'ERR:' + e.constructor.name}})()", quickjs.EvalGlobal)
	if err != nil {
		return "EVAL FAILED: " + err.Error()
	}

	s, _ := v.(string)
	return s
}

// goldenLocales are the locales the corpus formats in: the ones tifa users
// are likely to write, plus enough others to cover Indian grouping, other
// digits, right to left text, a minimum grouping of two and plural rules
// with every category.
var goldenLocales = []string{
	"en-US", "en-GB", "en-IN", "zh-CN", "zh-TW", "zh-HK", "ja", "ko", "de", "de-CH", "fr", "fr-CA",
	"fr-CH", "es", "es-MX", "pt-BR", "pt-PT", "it", "ru", "pl", "uk", "cs", "nl", "sv", "tr", "ar",
	"ar-EG", "he", "fa", "hi", "bn", "th", "vi", "id",
}

func jsList(s ...string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// goldenExprs returns the expressions to record and compare.
func goldenExprs() (r []string) {
	add := func(format string, args ...any) { r = append(r, fmt.Sprintf(format, args...)) }

	// Locale negotiation.
	for _, tag := range []string{
		"zh", "zh-CN", "zh-Hans", "zh-Hans-CN", "zh-Hant", "zh-TW", "zh-Hant-TW", "zh-HK", "zh-SG", "zh-MO",
		"en", "en-XX", "en-GB", "en-001", "en-u-nu-arab", "en-US-u-hc-h23", "ar-EG-u-nu-latn", "ar-u-nu-arab",
		"de-u-co-phonebk", "ja-JP-u-ca-japanese", "th-TH", "th-u-nu-thai", "cy", "eu", "yue", "iw", "in",
		"nb", "no", "sr", "sr-Latn", "sr-Latn-RS", "pt", "pt-PT", "es-419", "es-AR", "hi-Latn", "x-private",
		"EN-us", "zh-hant-tw", "fil", "tl", "sh", "mo",
	} {
		for _, c := range []string{"NumberFormat", "DateTimeFormat", "PluralRules"} {
			add(`new Intl.%s(%q).resolvedOptions().locale`, c, tag)
		}
	}
	add(`JSON.stringify(Intl.getCanonicalLocales(%s))`, jsList("EN-us", "zh-hant-tw", "iw", "in-ID", "en-u-ca-gregory-nu-latn", "de-DE-u-co-phonebk", "sr-cyrl-rs", "und", "en-GB-oed"))
	add(`JSON.stringify(Intl.NumberFormat.supportedLocalesOf(%s))`, jsList("zh-CN", "cy", "fr-CA", "yue", "ar-EG", "eu"))
	add(`JSON.stringify(Intl.DateTimeFormat.supportedLocalesOf(%s))`, jsList("zh-TW", "cy", "ne-IN", "ja-JP"))
	add(`JSON.stringify(Intl.PluralRules.supportedLocalesOf(%s))`, jsList("cy", "yue", "gd", "en-XX"))

	// Numbers.
	numberOptions := []string{
		`{}`, `{maximumFractionDigits:2}`, `{minimumFractionDigits:2}`, `{maximumSignificantDigits:3}`,
		`{style:'percent'}`, `{style:'percent',minimumFractionDigits:1}`,
		`{style:'currency',currency:'USD'}`, `{style:'currency',currency:'EUR'}`, `{style:'currency',currency:'CNY'}`,
		`{style:'currency',currency:'JPY'}`, `{style:'currency',currency:'TWD'}`, `{style:'currency',currency:'INR'}`,
		`{style:'currency',currency:'GBP',currencyDisplay:'code'}`, `{style:'currency',currency:'USD',currencyDisplay:'narrowSymbol'}`,
		`{style:'currency',currency:'EUR',currencyDisplay:'name'}`, `{style:'currency',currency:'USD',currencySign:'accounting'}`,
		`{style:'currency',currency:'KRW'}`, `{style:'currency',currency:'CHF'}`,
		`{style:'unit',unit:'kilometer'}`, `{style:'unit',unit:'kilometer',unitDisplay:'long'}`,
		`{style:'unit',unit:'kilometer-per-hour'}`, `{style:'unit',unit:'liter',unitDisplay:'long'}`,
		`{style:'unit',unit:'celsius'}`, `{style:'unit',unit:'megabyte'}`, `{style:'unit',unit:'day',unitDisplay:'long'}`,
		`{style:'unit',unit:'hour',unitDisplay:'narrow'}`, `{style:'unit',unit:'percent'}`,
		`{style:'unit',unit:'gigabyte-per-second'}`, `{style:'unit',unit:'kilogram',unitDisplay:'narrow'}`,
		`{notation:'compact'}`, `{notation:'compact',compactDisplay:'long'}`,
		`{notation:'compact',style:'currency',currency:'USD'}`,
		`{notation:'scientific'}`, `{notation:'engineering'}`,
		`{signDisplay:'always'}`, `{signDisplay:'exceptZero'}`,
		`{useGrouping:false}`, `{useGrouping:'min2'}`, `{useGrouping:'always'}`,
		`{numberingSystem:'arab'}`, `{numberingSystem:'latn'}`, `{numberingSystem:'hanidec'}`,
	}
	values := []string{"0", "-0", "1", "2", "5", "21", "1.5", "-1234.5678", "1234567.891", "0.005", "999999", "12345", "1e21", "NaN", "-Infinity"}
	for _, l := range goldenLocales {
		for _, o := range numberOptions {
			add(`JSON.stringify(new Intl.NumberFormat(%q,%s).resolvedOptions())`, l, o)
			for _, v := range values {
				add(`new Intl.NumberFormat(%q,%s).format(%s)`, l, o, v)
			}
			add(`JSON.stringify(new Intl.NumberFormat(%q,%s).formatToParts(-1234567.891))`, l, o)
		}
		add(`(1234.5).toLocaleString(%q)`, l)
		add(`(123456789n).toLocaleString(%q)`, l)
	}
	add(`(1234.5).toLocaleString()`)

	// Dates.
	dateOptions := []string{
		`{}`,
		`{dateStyle:'full'}`, `{dateStyle:'long'}`, `{dateStyle:'medium'}`, `{dateStyle:'short'}`,
		`{timeStyle:'full'}`, `{timeStyle:'long'}`, `{timeStyle:'medium'}`, `{timeStyle:'short'}`,
		`{dateStyle:'full',timeStyle:'short'}`, `{dateStyle:'long',timeStyle:'medium'}`,
		`{dateStyle:'medium',timeStyle:'short'}`, `{dateStyle:'short',timeStyle:'short'}`,
		`{dateStyle:'medium',timeStyle:'short',hour12:true}`, `{dateStyle:'short',timeStyle:'long',hourCycle:'h23'}`,
		`{year:'numeric',month:'long',day:'numeric'}`, `{year:'numeric',month:'short',day:'numeric'}`,
		`{year:'numeric',month:'2-digit',day:'2-digit'}`, `{year:'2-digit',month:'numeric',day:'numeric'}`,
		`{month:'short',day:'numeric'}`, `{month:'long',day:'numeric'}`, `{month:'numeric',day:'numeric'}`,
		`{year:'numeric',month:'long'}`, `{year:'numeric',month:'numeric'}`, `{year:'numeric'}`,
		`{month:'long'}`, `{month:'narrow'}`, `{day:'numeric'}`,
		`{weekday:'long'}`, `{weekday:'short'}`, `{weekday:'narrow'}`,
		`{weekday:'long',year:'numeric',month:'long',day:'numeric'}`, `{weekday:'short',month:'short',day:'numeric'}`,
		`{weekday:'short',hour:'numeric',minute:'numeric'}`,
		`{era:'short',year:'numeric'}`, `{era:'long',year:'numeric',month:'long',day:'numeric'}`,
		`{hour:'numeric'}`, `{hour:'numeric',minute:'numeric'}`, `{hour:'2-digit',minute:'2-digit'}`,
		`{hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{hour:'numeric',minute:'numeric',hour12:true}`, `{hour:'numeric',minute:'numeric',hour12:false}`,
		`{hour:'numeric',minute:'numeric',hourCycle:'h11'}`, `{hour:'numeric',minute:'numeric',hourCycle:'h23'}`,
		`{minute:'numeric',second:'numeric'}`, `{minute:'2-digit',second:'2-digit',fractionalSecondDigits:3}`,
		`{hour:'numeric',dayPeriod:'short'}`, `{dayPeriod:'long'}`, `{hour:'numeric',minute:'numeric',dayPeriod:'narrow'}`,
		`{timeZoneName:'short'}`, `{timeZoneName:'long'}`, `{timeZoneName:'shortOffset'}`, `{timeZoneName:'longOffset'}`,
		`{timeZoneName:'shortGeneric'}`, `{timeZoneName:'longGeneric'}`,
		`{hour:'numeric',minute:'numeric',timeZoneName:'short'}`,
		`{year:'numeric',month:'numeric',day:'numeric',hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{month:'long',day:'numeric',hour:'numeric',minute:'numeric'}`,
	}
	instants := []string{"0", "1755324309123", "1741521600000", "1767225599999", "-62198755200000"}
	zones := []string{"UTC", "Asia/Shanghai", "America/New_York", "Europe/Berlin", "Asia/Kolkata", "Asia/Tokyo", "America/Sao_Paulo"}
	for li, l := range goldenLocales {
		for oi, o := range dateOptions {
			zone := zones[(li+oi)%len(zones)]
			opts := strings.TrimSuffix(o, "}")
			if opts != "{" {
				opts += ","
			}
			opts += "timeZone:'" + zone + "'}"
			add(`JSON.stringify(new Intl.DateTimeFormat(%q,%s).resolvedOptions())`, l, opts)
			for _, at := range instants {
				add(`new Intl.DateTimeFormat(%q,%s).format(%s)`, l, opts, at)
			}
			add(`JSON.stringify(new Intl.DateTimeFormat(%q,%s).formatToParts(1755324309123))`, l, opts)
		}
		for _, m := range []string{"toLocaleString", "toLocaleDateString", "toLocaleTimeString"} {
			add(`new Date(1755324309123).%s(%q)`, m, l)
			add(`new Date(1755324309123).%s(%q,{timeZone:'Asia/Shanghai'})`, m, l)
		}
		// Every hour of a day, for the day periods and the hour cycle.
		for h := 0; h < 24; h += 3 {
			add(`new Intl.DateTimeFormat(%q,{hour:'numeric',minute:'numeric',timeZone:'UTC'}).format(%d)`, l, h*3600000+1800000)
			add(`new Intl.DateTimeFormat(%q,{hour:'numeric',dayPeriod:'long',timeZone:'UTC'}).format(%d)`, l, h*3600000)
		}
	}
	for _, m := range []string{"toLocaleString", "toLocaleDateString", "toLocaleTimeString"} {
		add(`new Date(1755324309123).%s()`, m)
	}
	add(`JSON.stringify(new Intl.DateTimeFormat().resolvedOptions())`)

	// Hour cycle keywords, numbering systems and calendars in the locale.
	for _, l := range []string{
		"en-US-u-hc-h23", "en-GB-u-hc-h12", "ja-JP-u-hc-h11", "zh-CN-u-hc-h12", "de-u-hc-h24", "ko-KR-u-hc-h23",
		"ja-JP-u-ca-japanese", "zh-TW-u-ca-roc", "en-u-ca-buddhist", "th-u-ca-gregory", "fa-u-ca-gregory",
		"ar-SA", "ar-u-ca-islamic", "he-u-ca-hebrew", "zh-u-ca-chinese", "en-u-ca-persian", "th-TH-u-nu-thai",
		"hi-u-nu-deva", "zh-u-nu-hanidec", "ar-EG-u-nu-latn", "fa-AF", "ps", "my", "ne", "mr",
	} {
		for _, o := range []string{
			`{}`, `{dateStyle:'full'}`, `{dateStyle:'medium',timeStyle:'short'}`, `{timeStyle:'full'}`,
			`{hour:'numeric',minute:'numeric'}`, `{hour:'numeric',hour12:true}`, `{hour:'numeric',hour12:false}`,
			`{hour:'numeric',hourCycle:'h23'}`, `{hour:'numeric',hourCycle:'h11'}`, `{year:'numeric',era:'short'}`,
			`{year:'numeric',month:'long'}`, `{calendar:'gregory',dateStyle:'long'}`,
		} {
			opts := strings.TrimSuffix(o, "}")
			if opts != "{" {
				opts += ","
			}
			opts += "timeZone:'UTC'}"
			add(`JSON.stringify(new Intl.DateTimeFormat(%q,%s).resolvedOptions())`, l, opts)
			add(`new Intl.DateTimeFormat(%q,%s).format(1755324309123)`, l, opts)
			add(`new Intl.DateTimeFormat(%q,%s).format(0)`, l, opts)
		}
	}

	// Years before AD 1 and at the ends of the range.
	for _, l := range []string{"en-US", "zh-CN", "ja", "de", "th", "fa", "ar"} {
		for _, o := range []string{`{}`, `{era:'short',year:'numeric'}`, `{dateStyle:'full'}`, `{weekday:'short',year:'numeric',month:'short',day:'numeric'}`} {
			opts := strings.TrimSuffix(o, "}")
			if opts != "{" {
				opts += ","
			}
			opts += "timeZone:'UTC'}"
			for _, at := range []string{"-77931370800000", "-8.64e15", "8.64e15", "-62135596800001"} {
				add(`new Intl.DateTimeFormat(%q,%s).format(%s)`, l, opts, at)
			}
		}
	}

	// Time zone names: many zones, in the locales with the most names.
	nameZones := []string{
		"America/Los_Angeles", "America/Chicago", "America/Denver", "America/Phoenix", "America/Anchorage",
		"Pacific/Honolulu", "America/Toronto", "America/Mexico_City", "America/Argentina/Buenos_Aires",
		"America/St_Johns", "Europe/London", "Europe/Dublin", "Europe/Paris", "Europe/Moscow", "Europe/Istanbul",
		"Africa/Cairo", "Africa/Casablanca", "Africa/Lagos", "Asia/Jerusalem", "Asia/Tehran", "Asia/Dubai",
		"Asia/Karachi", "Asia/Kathmandu", "Asia/Dhaka", "Asia/Bangkok", "Asia/Ho_Chi_Minh", "Asia/Jakarta",
		"Asia/Singapore", "Asia/Hong_Kong", "Asia/Taipei", "Asia/Seoul", "Australia/Sydney", "Australia/Perth",
		"Pacific/Auckland", "Pacific/Chatham", "Asia/Macau", "Africa/Abidjan", "Atlantic/Reykjavik", "Asia/Calcutta", "US/Eastern", "Etc/GMT+5", "Etc/UTC", "GMT",
		"+05:30", "-08:00",
	}
	for _, l := range []string{"en-US", "en-GB", "zh-CN", "zh-TW", "ja", "de", "fr", "es", "pt-BR", "ru", "ko", "ar"} {
		for _, z := range nameZones {
			for _, n := range []string{"short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric"} {
				for _, at := range []string{"1755324309123", "1767225599999"} {
					add(`new Intl.DateTimeFormat(%q,{timeZoneName:'%s',timeZone:'%s'}).format(%s)`, l, n, z, at)
				}
			}
		}
	}
	for _, z := range nameZones {
		add(`new Intl.DateTimeFormat('en',{timeZone:'%s'}).resolvedOptions().timeZone`, z)
	}

	// Date errors.
	for _, src := range []string{
		`new Intl.DateTimeFormat('en',{dateStyle:'short',hour:'numeric'})`,
		`new Date(0).toLocaleDateString('en',{timeStyle:'short'})`,
		`new Date(0).toLocaleTimeString('en',{dateStyle:'short'})`,
		`new Intl.DateTimeFormat('en').format(NaN)`, `new Intl.DateTimeFormat('en').format(8.64e15+1)`,
		`new Intl.DateTimeFormat('en',{calendar:'x'})`, `new Intl.DateTimeFormat('en',{hourCycle:'h25'})`,
		`new Intl.DateTimeFormat('en',{fractionalSecondDigits:4})`, `new Intl.DateTimeFormat('en',{dateStyle:'tiny'})`,
		`new Intl.DateTimeFormat('en',{timeZone:'+25:00'})`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}

	r = append(r, enUSExprs()...)

	// Plural rules.
	pluralValues := []string{"0", "1", "2", "3", "4", "5", "6", "7", "11", "12", "21", "22", "100", "101", "1000000", "0.5", "1.5", "2.5", "-1"}
	for _, l := range []string{"en", "fr", "ru", "pl", "cs", "ar", "he", "cy", "lt", "ja", "zh", "ko", "de", "pt", "pt-PT", "es", "it", "hi", "uk", "ga", "sl"} {
		for _, o := range []string{`{}`, `{type:'ordinal'}`, `{minimumFractionDigits:1}`, `{maximumSignificantDigits:1}`} {
			add(`JSON.stringify(new Intl.PluralRules(%q,%s).resolvedOptions())`, l, o)
			for _, v := range pluralValues {
				add(`new Intl.PluralRules(%q,%s).select(%s)`, l, o, v)
			}
		}
		add(`new Intl.PluralRules(%q).selectRange(1,2)`, l)
		add(`new Intl.PluralRules(%q).selectRange(0,1)`, l)
		add(`new Intl.PluralRules(%q).selectRange(1,5)`, l)
	}

	// Errors.
	for _, src := range []string{
		`new Intl.NumberFormat('en-')`, `new Intl.NumberFormat('a')`, `new Intl.NumberFormat('en-US-u')`,
		`new Intl.NumberFormat('en',{style:'unit',unit:'furlong'})`, `new Intl.NumberFormat('en',{style:'unit'})`,
		`new Intl.NumberFormat('en',{numberingSystem:'xx'})`, `new Intl.DateTimeFormat('en',{timeZone:'Mars/Olympus'})`,
		`new Intl.PluralRules('en',{type:'cardinalish'})`, `Intl.PluralRules('en')`,
	} {
		add(`(() => {try {%s; return 'ok'} catch (e) {return e.constructor.name}})()`, src)
	}

	seen := map[string]bool{}
	out := r[:0]
	for _, e := range r {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// enUSExprs returns en-US in depth: every option across values and instants
// chosen to hit the edges, a DST switch in both directions, a half hour and a
// three quarter hour zone, a value that rounds up into another magnitude,
// zero, NaN and both infinities.
func enUSExprs() (r []string) {
	dateOptions := []string{
		`{}`,
		`{dateStyle:'full'}`, `{dateStyle:'long'}`, `{dateStyle:'medium'}`, `{dateStyle:'short'}`,
		`{timeStyle:'full'}`, `{timeStyle:'long'}`, `{timeStyle:'medium'}`, `{timeStyle:'short'}`,
		`{dateStyle:'full',timeStyle:'full'}`, `{dateStyle:'long',timeStyle:'medium'}`,
		`{dateStyle:'medium',timeStyle:'short'}`, `{dateStyle:'short',timeStyle:'short'}`,
		`{year:'numeric'}`, `{year:'2-digit'}`, `{month:'numeric'}`, `{month:'2-digit'}`,
		`{month:'long'}`, `{month:'short'}`, `{month:'narrow'}`, `{day:'numeric'}`,
		`{weekday:'long'}`, `{weekday:'short'}`, `{weekday:'narrow'}`,
		`{year:'numeric',month:'long'}`, `{year:'numeric',month:'numeric'}`,
		`{month:'numeric',day:'numeric'}`, `{month:'long',day:'numeric'}`,
		`{year:'numeric',month:'long',day:'numeric'}`, `{year:'numeric',month:'short',day:'numeric'}`,
		`{year:'numeric',month:'2-digit',day:'2-digit'}`,
		`{weekday:'long',year:'numeric',month:'long',day:'numeric'}`,
		`{weekday:'short',month:'numeric',day:'numeric'}`,
		`{year:'numeric',era:'short'}`, `{year:'numeric',era:'long'}`,
		`{hour:'numeric'}`, `{hour:'2-digit'}`, `{hour:'numeric',minute:'numeric'}`,
		`{hour:'numeric',minute:'2-digit'}`, `{hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{hour:'numeric',minute:'numeric',hour12:false}`, `{hour:'numeric',minute:'numeric',hour12:true}`,
		`{hour:'numeric',minute:'numeric',hourCycle:'h23'}`, `{hour:'numeric',minute:'numeric',hourCycle:'h11'}`,
		`{hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}`,
		`{hour:'numeric',minute:'numeric',second:'numeric',fractionalSecondDigits:3}`,
		`{hour:'numeric',minute:'numeric',second:'numeric',fractionalSecondDigits:1}`,
		`{minute:'numeric',second:'numeric'}`,
		`{timeZoneName:'short'}`, `{timeZoneName:'shortOffset'}`, `{timeZoneName:'longOffset'}`,
		`{hour:'numeric',timeZoneName:'short'}`,
		`{year:'numeric',month:'numeric',day:'numeric',hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{month:'long',day:'numeric',hour:'numeric'}`, `{month:'short',day:'numeric',hour:'numeric'}`,
		`{dayPeriod:'short'}`, `{hour:'numeric',dayPeriod:'short'}`,
	}
	instants := []string{
		"0",
		"1000000000000",
		"1755300000000",
		"1741503600000", // 2025-03-09T07:00:00Z, a US DST switch.
		"1762063200000", // 2025-11-02T06:00:00Z, back again.
		"1500",
		"-1000000000000",
	}
	zones := []string{
		"'UTC'", "'America/New_York'", "'Europe/Prague'",
		"'Asia/Kolkata'",     // A half hour offset.
		"'Australia/Sydney'", // DST the other way around.
		"'Pacific/Chatham'",  // Three quarters of an hour.
	}
	for i, opts := range dateOptions {
		for j, at := range instants {
			o := strings.TrimSuffix(opts, "}")
			if o != "{" {
				o += ","
			}
			o += "timeZone:" + zones[(i+j)%len(zones)] + "}"
			r = append(r,
				fmt.Sprintf(`new Intl.DateTimeFormat('en-US',%s).format(new Date(%s))`, o, at),
				fmt.Sprintf(`JSON.stringify(new Intl.DateTimeFormat('en-US',%s).formatToParts(new Date(%s)))`, o, at),
				fmt.Sprintf(`JSON.stringify(new Intl.DateTimeFormat('en-US',%s).resolvedOptions())`, o),
			)
		}
	}
	for _, opts := range dateOptions {
		for _, m := range []string{"toLocaleString", "toLocaleDateString", "toLocaleTimeString"} {
			r = append(r, fmt.Sprintf(`new Date(1755300000000).%s('en-US',%s)`, m, opts))
		}
	}

	numberOptions := []string{
		`{}`, `{useGrouping:false}`, `{useGrouping:'min2'}`, `{useGrouping:'always'}`,
		`{minimumFractionDigits:2}`, `{maximumFractionDigits:0}`, `{maximumFractionDigits:1}`,
		`{maximumFractionDigits:5}`, `{minimumFractionDigits:2,maximumFractionDigits:4}`,
		`{minimumIntegerDigits:3}`, `{minimumSignificantDigits:3}`, `{maximumSignificantDigits:2}`,
		`{maximumSignificantDigits:4}`, `{minimumSignificantDigits:2,maximumSignificantDigits:5}`,
		`{style:'percent'}`, `{style:'percent',maximumFractionDigits:2}`, `{style:'percent',minimumFractionDigits:1}`,
		`{style:'currency',currency:'USD'}`, `{style:'currency',currency:'EUR'}`,
		`{style:'currency',currency:'JPY'}`, `{style:'currency',currency:'GBP'}`,
		`{style:'currency',currency:'CAD'}`, `{style:'currency',currency:'CAD',currencyDisplay:'narrowSymbol'}`,
		`{style:'currency',currency:'USD',currencyDisplay:'code'}`,
		`{style:'currency',currency:'USD',currencySign:'accounting'}`, `{style:'currency',currency:'BHD'}`,
		`{style:'unit',unit:'liter'}`, `{style:'unit',unit:'kilometer'}`,
		`{style:'unit',unit:'kilometer-per-hour'}`, `{style:'unit',unit:'byte'}`,
		`{style:'unit',unit:'megabyte',unitDisplay:'long'}`, `{style:'unit',unit:'day',unitDisplay:'long'}`,
		`{style:'unit',unit:'celsius',unitDisplay:'narrow'}`,
		`{notation:'compact'}`, `{notation:'compact',compactDisplay:'long'}`,
		`{notation:'scientific'}`, `{notation:'engineering'}`,
		`{signDisplay:'always'}`, `{signDisplay:'never'}`, `{signDisplay:'exceptZero'}`, `{signDisplay:'negative'}`,
		`{maximumFractionDigits:0,roundingMode:'ceil'}`, `{maximumFractionDigits:0,roundingMode:'floor'}`,
		`{maximumFractionDigits:0,roundingMode:'expand'}`, `{maximumFractionDigits:0,roundingMode:'trunc'}`,
		`{maximumFractionDigits:0,roundingMode:'halfCeil'}`, `{maximumFractionDigits:0,roundingMode:'halfFloor'}`,
		`{maximumFractionDigits:0,roundingMode:'halfTrunc'}`, `{maximumFractionDigits:0,roundingMode:'halfEven'}`,
		`{maximumFractionDigits:2,roundingMode:'halfEven'}`, `{maximumFractionDigits:2,roundingMode:'expand'}`,
		`{minimumFractionDigits:2,trailingZeroDisplay:'stripIfInteger'}`,
		`{maximumSignificantDigits:3,roundingMode:'trunc'}`,
	}
	values := []string{
		"0", "-0", "1", "-1", "0.5", "1.005", "2.5", "3.5", "1234.5678", "1234567.891",
		"999999", "1000", "1e21", "1e-7", "123456789", "-1234.5", "NaN", "Infinity",
		"-Infinity", "0.0001", "12345", "99.995", "1e6", "1e9",
	}
	for _, opts := range numberOptions {
		for _, v := range values {
			r = append(r,
				fmt.Sprintf(`new Intl.NumberFormat('en-US',%s).format(%s)`, opts, v),
				fmt.Sprintf(`(%s).toLocaleString('en-US',%s)`, v, opts),
			)
		}
		r = append(r,
			fmt.Sprintf(`JSON.stringify(new Intl.NumberFormat('en-US',%s).resolvedOptions())`, opts),
			fmt.Sprintf(`JSON.stringify(new Intl.NumberFormat('en-US',%s).formatToParts(-1234.5678))`, opts),
		)
	}
	return r
}
