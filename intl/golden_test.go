// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

// TestGolden compares what Intl answers with what Chromium answered for the
// same expressions, recorded in testdata/chromium. To record again: go test
// -run TestGolden -golden.corpus writes the expressions of each set to
// corpus*.json, and node intl/testdata/chromium/record.cjs golden evaluates
// them in Chromium and writes golden*.json.gz.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/liustack/slowjs"
)

var (
	goldenCorpus = flag.Bool("golden.corpus", false, "write the expressions of each golden set to testdata/chromium/corpus*.json and skip the comparison")
	goldenRun    = flag.String("golden.run", "", "compare only the golden expressions that match this regular expression")
	goldenShow   = flag.Int("golden.show", 40, "report at most this many unexplained golden differences")
)

// goldenDir holds what Chromium answered.
var goldenDir = filepath.Join("testdata", "chromium")

// goldenLocales are the locales the base set and numrange set ask about:
// those tifa's users write most, and others for Indian grouping, other
// digits, right-to-left scripts, a minimum grouping of 2, plural rules with
// every category, currencies with patterns of their own (en-DE), ties of
// date patterns (en-AU), and a language Chromium has no data for (az). The
// order is fixed: the numrange corpus follows it.
var goldenLocales = []string{
	"en-US", "en-GB", "en-IN", "en-AU", "en-DE", "zh-CN", "zh-TW", "zh-HK", "ja", "ko", "de", "de-CH",
	"fr", "fr-CA", "fr-CH", "es", "es-MX", "pt-BR", "pt-PT", "it", "ru", "pl", "uk", "cs", "nl", "sv",
	"tr", "ar", "ar-EG", "he", "fa", "hi", "bn", "th", "vi", "id", "az",
}

// jsList is s as a javascript array of strings.
func jsList(s ...string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// goldenEval is expr as the recorder writes it down: its string, or ERR:
// and the constructor of what it throws.
func goldenEval(t *testing.T, vm *slowjs.VM, expr string) string {
	t.Helper()
	v, err := vm.Eval("(() => {try {return String("+expr+")} catch (e) {return 'ERR:' + e.constructor.name}})()", slowjs.EvalGlobal)
	if err != nil {
		return "EVAL FAILED: " + err.Error()
	}
	s, _ := v.(string)
	return s
}

// goldenSets are the recorded sets, by name, and what writes the expressions
// of each.
var goldenSets = []struct {
	name  string
	exprs func() []string
}{
	{"", goldenExprs},
	{"collator", collatorExprs},
	{"segmenter", segmenterExprs},
	{"relativetime", rtfExprs},
	{"list", listExprs},
	{"displaynames", dnExprs},
	{"duration", durationExprs},
	{"values", valuesExprs},
	{"locale", localeExprs},
	{"case", caseExprs},
	{"numrange", numrangeExprs},
}

// goldenFile is the file of the set name: golden.json.gz for the base set,
// golden-<name>.json.gz for the others, and the same with corpus and .json.
func goldenFile(prefix, name, ext string) string {
	if name != "" {
		prefix += "-" + name
	}
	return filepath.Join(goldenDir, prefix+ext)
}

// A goldenRecord is a set as the recorder writes it.
type goldenRecord struct {
	Browser   string      `json:"browser"`
	UserAgent string      `json:"userAgent"`
	Cases     [][2]string `json:"cases"`
}

func readGolden(name string) (*goldenRecord, error) {
	b, err := os.ReadFile(goldenFile("golden", name, ".json.gz"))
	if err != nil {
		return nil, err
	}

	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	defer z.Close()

	var r goldenRecord
	if err := json.NewDecoder(z).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// goldenExprs are the expressions of the base set: those recorded last, so
// that recording again changes the answers, not the questions.
func goldenExprs() []string {
	r, err := readGolden("")
	if err != nil {
		panic(err)
	}

	exprs := make([]string, len(r.Cases))
	for i, c := range r.Cases {
		exprs[i] = c[0]
	}
	return exprs
}

func writeCorpus(t *testing.T) {
	for _, set := range goldenSets {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "\t")
		if err := enc.Encode(set.exprs()); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile("corpus", set.name, ".json"), b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGolden(t *testing.T) {
	if *goldenCorpus {
		writeCorpus(t)
		t.Skip("wrote the corpus; record it with node intl/testdata/chromium/record.cjs golden")
	}

	var only *regexp.Regexp
	if *goldenRun != "" {
		only = regexp.MustCompile(*goldenRun)
	}

	for _, set := range goldenSets {
		name := set.name
		if name == "" {
			name = "base"
		}
		t.Run(name, func(t *testing.T) {
			r, err := readGolden(set.name)
			if err != nil {
				t.Fatal(err)
			}

			recorded := make(map[string]string, len(r.Cases))
			for _, c := range r.Cases {
				recorded[c[0]] = c[1]
			}

			vm := newVM(t)
			same, total, unexplained := 0, 0, 0
			reasons := map[string]int{}
			for _, c := range r.Cases {
				expr, want := c[0], c[1]
				if only != nil && !only.MatchString(expr) {
					continue
				}

				total++
				got := goldenEval(t, vm, expr)
				if got == want {
					same++
					continue
				}

				if reason := goldenDifferences(expr, got, want, recorded); reason != "" {
					reasons[reason]++
					continue
				}

				unexplained++
				if unexplained <= *goldenShow {
					t.Errorf("%s\n got %q\nwant %q", expr, got, want)
				}
			}
			if unexplained > *goldenShow {
				t.Errorf("... and %d more", unexplained-*goldenShow)
			}

			explained := 0
			for _, reason := range slices.Sorted(maps.Keys(reasons)) {
				t.Logf("explained, %s: %d", reason, reasons[reason])
				explained += reasons[reason]
			}
			t.Logf("%s: %d of %d the same, %d explained, %d unexplained", r.Browser, same, total, explained, unexplained)
		})
	}
}

// goldenDifferences is the reason got may differ from Chromium's want, or ""
// when no known difference accounts for it. recorded holds every answer of
// the set.
func goldenDifferences(expr, got, want string, recorded map[string]string) string {
	switch {
	case strings.Contains(expr, `granularity:"word"`) && sameWords(got, want):
		// Chromium changed ICU's word rules: a period or a colon parts
		// letters into words. And it breaks Lao and Khmer with dictionaries
		// of its own, smaller ones.
		return "word breaks"
	case goldenOtherCalendar(expr) && calendarDropped(expr, got, recorded):
		return "calendar without data, formatted as gregory"
	}

	if items, ok := dnItems(expr, got, want); ok && len(items) > 0 {
		english := dnEnglish(expr, recorded)
		if all(items, func(it dnItem) bool { return dnExplained(it, english) }) {
			// Chromium's ICU data has fewer names, and some English names
			// of its own.
			return "display names"
		}
		if all(items, func(it dnItem) bool { return dnGoIntlFields[[3]string{it.loc, it.style, it.code}] }) {
			// The narrow names of de-CH's era and th's time zone fall back
			// otherwise in go-intl than in ICU.
			return "narrow field names"
		}
	}

	switch {
	case strings.Contains(expr, "Intl.DurationFormat") &&
		durationOverflow.ReplaceAllString(got, "${1}807") == want:
		// V8 turns a sum of nanoseconds past 2**63 into an int64 as the
		// machine does: INT64_MIN on x86-64, INT64_MAX on the arm64 the
		// recording ran on.
		return "duration past 2**63"
	case expr == `Intl.supportedValuesOf("currency").join()` && chromiumCurrencies(got) == want &&
		want == strings.Join(goldenChromiumCurrencies(), ","):
		return "currencies Chromium has no data for"
	case localeDiffers(expr, got, want, func(i int, g, w string) bool {
		return i == 19 && g == withoutZones(w, "CST6CDT", "EST5EDT", "Factory", "MST7MDT", "PST8PDT")
	}):
		return "Locale time zones of region 001"
	case localeDiffers(expr, got, want, func(i int, g, w string) bool {
		return i == 1 && w == g+"-u-va-posix" || i == 5 && w == "posix"
	}):
		return "Locale -u-va-posix"
	case (strings.Contains(expr, `Locale("en-u-rg-xxzzzz")`) || strings.Contains(expr, `Locale("und-ZZ")`) ||
		strings.Contains(expr, `Locale("und-Zzzz-ZZ")`)) &&
		localeDiffers(expr, got, want, func(i int, g, w string) bool { return i == 17 || i == 21 }):
		return "Locale of an unknown region"
	}
	return ""
}

func all[T any](l []T, f func(T) bool) bool {
	for _, x := range l {
		if !f(x) {
			return false
		}
	}
	return true
}

var durationOverflow = regexp.MustCompile(`-?(9223372036[.,]854775)808`)

// sameWords reports whether got and want, each a list of segments, break
// a text alike but for the word rules Chromium changed.
func sameWords(got, want string) bool {
	g, ok := wordBreaks(got)
	if !ok {
		return false
	}
	w, ok := wordBreaks(want)
	return ok && g == w
}

// wordBreaks writes the segments of s, a json list of [segment, index,
// isWordLike], with the words as Chromium would break them: the word-like
// segments with Lao or Khmer in a row as one, and every other word-like
// segment parted at each period and colon, the mark a segment of its own.
func wordBreaks(s string) (string, bool) {
	var segs [][3]any
	if err := json.Unmarshal([]byte(s), &segs); err != nil {
		return "", false
	}

	var out []string
	run := ""
	inRun := false
	for _, seg := range segs {
		text, ok := seg[0].(string)
		if !ok {
			return "", false
		}

		wordLike, _ := seg[2].(bool)
		if wordLike && laoOrKhmer(text) {
			run += text
			inRun = true
			continue
		}

		if inRun {
			out = append(out, run)
			run, inRun = "", false
		}
		if !wordLike {
			out = append(out, text)
			continue
		}

		start := 0
		for i, r := range text {
			if r == '.' || r == ':' {
				out = append(out, text[start:i], string(r))
				start = i + 1
			}
		}
		out = append(out, text[start:])
	}
	if inRun {
		out = append(out, run)
	}
	return strings.Join(out, "|"), true
}

func laoOrKhmer(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Lao, r) || unicode.Is(unicode.Khmer, r) {
			return true
		}
	}
	return false
}

// calendarDropped reports whether got is what Chromium answered for expr
// without its calendar.
func calendarDropped(expr, got string, recorded map[string]string) bool {
	want, ok := recorded[goldenDropCalendar(expr)]
	return ok && got == want
}

var (
	calendarKey      = regexp.MustCompile(`-u-ca-(\w+)`)
	calendarKeyAhead = regexp.MustCompile(`-u-ca-(\w+)-`)
	calendarOption   = regexp.MustCompile(`calendar:'(\w+)'`)
	calendarField    = regexp.MustCompile(`,?calendar:'(\w+)',?`)
)

// goldenOtherCalendar reports whether expr asks for a calendar other than
// gregory, in a tag or in an option.
func goldenOtherCalendar(expr string) bool {
	for _, re := range []*regexp.Regexp{calendarKey, calendarOption} {
		for _, m := range re.FindAllStringSubmatch(expr, -1) {
			if m[1] != "gregory" {
				return true
			}
		}
	}
	return false
}

// goldenDropCalendar is expr without the calendars other than gregory it
// asks for.
func goldenDropCalendar(expr string) string {
	expr = calendarKeyAhead.ReplaceAllStringFunc(expr, func(m string) string {
		if calendarKeyAhead.FindStringSubmatch(m)[1] == "gregory" {
			return m
		}
		return "-u-"
	})
	expr = calendarKey.ReplaceAllStringFunc(expr, func(m string) string {
		if calendarKey.FindStringSubmatch(m)[1] == "gregory" {
			return m
		}
		return ""
	})
	return calendarField.ReplaceAllStringFunc(expr, func(m string) string {
		if calendarField.FindStringSubmatch(m)[1] == "gregory" {
			return m
		}
		if strings.HasPrefix(m, ",") && strings.HasSuffix(m, ",") {
			return ","
		}
		return ""
	})
}

func TestGoldenDropCalendar(t *testing.T) {
	for _, tc := range [][2]string{
		{`new Intl.DateTimeFormat("ja-JP-u-ca-japanese",{timeZone:'UTC'})`, `new Intl.DateTimeFormat("ja-JP",{timeZone:'UTC'})`},
		{`new Intl.DateTimeFormat("en-u-ca-buddhist-hc-h23")`, `new Intl.DateTimeFormat("en-u-hc-h23")`},
		{`new Intl.DateTimeFormat("en-u-ca-gregory-nu-latn")`, `new Intl.DateTimeFormat("en-u-ca-gregory-nu-latn")`},
		{`new Intl.DateTimeFormat('en',{calendar:'roc'})`, `new Intl.DateTimeFormat('en',{})`},
		{`new Intl.DateTimeFormat('en',{calendar:'roc',year:'numeric'})`, `new Intl.DateTimeFormat('en',{year:'numeric'})`},
		{`new Intl.DateTimeFormat('en',{year:'numeric',calendar:'roc'})`, `new Intl.DateTimeFormat('en',{year:'numeric'})`},
		{`new Intl.DateTimeFormat('en',{year:'numeric',calendar:'roc',month:'long'})`, `new Intl.DateTimeFormat('en',{year:'numeric',month:'long'})`},
		{`new Intl.DateTimeFormat('en',{calendar:'gregory'})`, `new Intl.DateTimeFormat('en',{calendar:'gregory'})`},
	} {
		if got := goldenDropCalendar(tc[0]); got != tc[1] {
			t.Errorf("%s\n got %s\nwant %s", tc[0], got, tc[1])
		}
	}
}

// chromiumCurrencies is the list s of currencies with only those Chromium
// has data for.
func chromiumCurrencies(s string) string {
	known := map[string]bool{}
	for _, c := range goldenChromiumCurrencies() {
		known[c] = true
	}

	var kept []string
	for _, c := range strings.Split(s, ",") {
		if known[c] {
			kept = append(kept, c)
		}
	}
	return strings.Join(kept, ",")
}

// withoutZones is the list s of time zones without the zones drop.
func withoutZones(s string, drop ...string) string {
	var kept []string
	for _, z := range strings.Split(s, ",") {
		if !slices.Contains(drop, z) {
			kept = append(kept, z)
		}
	}
	return strings.Join(kept, ",")
}

// localeDiffers reports whether expr writes out a Locale with localeShow and
// every field where got and want differ is one that ok accepts, by its
// index: 1 is baseName, 5 variants, 17 hourCycles, 19 timeZones and 21
// weekInfo.
func localeDiffers(expr, got, want string, ok func(i int, got, want string) bool) bool {
	if !strings.HasPrefix(expr, localeShow) {
		return false
	}

	g, w := strings.Split(got, " | "), strings.Split(want, " | ")
	if len(g) != len(w) {
		return false
	}
	for i := range g {
		if g[i] != w[i] && !ok(i, g[i], w[i]) {
			return false
		}
	}
	return true
}
