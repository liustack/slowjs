// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"
	"strings"
)

// maxTimeValue is the largest magnitude of a javascript time value, ie. the
// range Date can represent.
const maxTimeValue = 8.64e15

// dateTimeFormat is a resolved Intl.DateTimeFormat. As with numberFormat its
// json form is both the resolvedOptions() result and the state the host gets
// back on every format call, so the fields are in the order ECMA-402 reports
// them, and the ones for the host only start with an underscore.
//
// Resolution follows V8 (js-date-time-format.cc): the options make an ICU
// skeleton, the pattern generator turns it into the locale's pattern, and
// resolvedOptions reads the components back from that pattern.
type dateTimeFormat struct {
	Locale                 string `json:"locale"`
	Calendar               string `json:"calendar"`
	NumberingSystem        string `json:"numberingSystem"`
	TimeZone               string `json:"timeZone"`
	HourCycle              string `json:"hourCycle,omitempty"`
	Hour12                 *bool  `json:"hour12,omitempty"`
	Weekday                string `json:"weekday,omitempty"`
	Era                    string `json:"era,omitempty"`
	Year                   string `json:"year,omitempty"`
	Month                  string `json:"month,omitempty"`
	Day                    string `json:"day,omitempty"`
	DayPeriod              string `json:"dayPeriod,omitempty"`
	Hour                   string `json:"hour,omitempty"`
	Minute                 string `json:"minute,omitempty"`
	Second                 string `json:"second,omitempty"`
	FractionalSecondDigits int    `json:"fractionalSecondDigits,omitempty"`
	TimeZoneName           string `json:"timeZoneName,omitempty"`
	DateStyle              string `json:"dateStyle,omitempty"`
	TimeStyle              string `json:"timeStyle,omitempty"`

	Pattern    string `json:"_pattern"`
	DataLocale string `json:"_data"`
	ZoneID     string `json:"_zone"`   // the CLDR id of the zone, "" for an offset
	Region     string `json:"_region"` // the locale's region, explicit or likely
	BaseLocale string `json:"_base"`   // the locale without extensions

	f *dateFormatter
}

var (
	numericWidths = []string{"2-digit", "numeric"}
	textWidths    = []string{"narrow", "long", "short"}
	styleWidths   = []string{"full", "long", "medium", "short"}
	styleIndex    = map[string]int{"full": 0, "long": 1, "medium": 2, "short": 3}
)

// component is a row of ECMA-402's table of date and time components with
// the skeleton letters V8 maps each value to.
type component struct {
	key     string
	allowed []string
	letters map[string]string
}

func components(hc string) []component {
	hour := map[string]string{"2-digit": "jj", "numeric": "j"}
	switch hc {
	case "h11":
		hour = map[string]string{"2-digit": "KK", "numeric": "K"}
	case "h12":
		hour = map[string]string{"2-digit": "hh", "numeric": "h"}
	case "h23":
		hour = map[string]string{"2-digit": "HH", "numeric": "H"}
	case "h24":
		hour = map[string]string{"2-digit": "kk", "numeric": "k"}
	}
	return []component{
		{"weekday", textWidths, map[string]string{"narrow": "EEEEE", "long": "EEEE", "short": "EEE"}},
		{"era", textWidths, map[string]string{"narrow": "GGGGG", "long": "GGGG", "short": "GGG"}},
		{"year", numericWidths, map[string]string{"2-digit": "yy", "numeric": "y"}},
		{"month", []string{"narrow", "long", "short", "2-digit", "numeric"},
			map[string]string{"narrow": "MMMMM", "long": "MMMM", "short": "MMM", "2-digit": "MM", "numeric": "M"}},
		{"day", numericWidths, map[string]string{"2-digit": "dd", "numeric": "d"}},
		{"dayPeriod", textWidths, map[string]string{"narrow": "BBBBB", "long": "BBBB", "short": "B"}},
		{"hour", numericWidths, hour},
		{"minute", numericWidths, map[string]string{"2-digit": "mm", "numeric": "m"}},
		{"second", numericWidths, map[string]string{"2-digit": "ss", "numeric": "s"}},
		{"timeZoneName", []string{"long", "short", "longOffset", "shortOffset", "longGeneric", "shortGeneric"},
			map[string]string{"long": "zzzz", "short": "z", "longOffset": "OOOO", "shortOffset": "O", "longGeneric": "vvvv", "shortGeneric": "v"}},
	}
}

// patternItems are the pattern substrings resolvedOptions recognizes, per
// component, in V8's order: the first one the pattern contains wins.
var patternItems = []struct {
	key   string
	pairs [][2]string
}{
	{"weekday", [][2]string{{"EEEEE", "narrow"}, {"EEEE", "long"}, {"EEE", "short"}, {"ccccc", "narrow"}, {"cccc", "long"}, {"ccc", "short"}}},
	{"era", [][2]string{{"GGGGG", "narrow"}, {"GGGG", "long"}, {"GGG", "short"}}},
	{"year", [][2]string{{"yy", "2-digit"}, {"y", "numeric"}, {"YY", "2-digit"}, {"Y", "numeric"}}},
	{"month", [][2]string{{"MMMMM", "narrow"}, {"MMMM", "long"}, {"MMM", "short"}, {"MM", "2-digit"}, {"M", "numeric"},
		{"LLLLL", "narrow"}, {"LLLL", "long"}, {"LLL", "short"}, {"LL", "2-digit"}, {"L", "numeric"}}},
	{"day", [][2]string{{"dd", "2-digit"}, {"d", "numeric"}}},
	{"dayPeriod", [][2]string{{"BBBBB", "narrow"}, {"bbbbb", "narrow"}, {"BBBB", "long"}, {"bbbb", "long"}, {"B", "short"}, {"b", "short"}}},
	{"hour", [][2]string{{"HH", "2-digit"}, {"H", "numeric"}, {"hh", "2-digit"}, {"h", "numeric"}, {"kk", "2-digit"}, {"k", "numeric"}, {"KK", "2-digit"}, {"K", "numeric"}}},
	{"minute", [][2]string{{"mm", "2-digit"}, {"m", "numeric"}}},
	{"second", [][2]string{{"ss", "2-digit"}, {"s", "numeric"}}},
	{"timeZoneName", [][2]string{{"zzzz", "long"}, {"z", "short"}, {"OOOO", "longOffset"}, {"O", "shortOffset"}, {"vvvv", "longGeneric"}, {"v", "shortGeneric"}}},
}

// timeSkeletons are the skeletons ICU regenerates a time style from when the
// locale asks for an hour cycle.
var timeSkeletons = [4]string{"jmmsszzzz", "jmmssz", "jmmss", "jmm"}

var hourCycles = []string{"h11", "h12", "h23", "h24"}

func resolveDateTimeFormat(req request, defaultZone string) (*dateTimeFormat, error) {
	r, err := resolveLocale(req.Locales, dateService)
	if err != nil {
		return nil, err
	}

	o := req.Options
	if o == nil {
		o = map[string]any{}
	}
	required, _ := o["_required"].(string)
	if required == "" {
		required = "any"
	}
	defaults, _ := o["_defaults"].(string)
	if defaults == "" {
		defaults = "date"
	}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	caOption := ""
	if v, ok := o["calendar"]; ok {
		s, _ := v.(string)
		if !wellFormedType(s) {
			return nil, rangeError("Invalid calendar : %s", s)
		}

		caOption = strings.ToLower(s)
	}
	nuOption, err := numberingSystemOption(o)
	if err != nil {
		return nil, err
	}

	hour12, hasHour12 := optBool(o, "hour12")
	hourCycle, err := optString(o, "hourCycle", hourCycles...)
	if err != nil {
		return nil, err
	}

	if hasHour12 {
		hourCycle = ""
	}

	// A calendar is supported where there is data for it: the Gregorian one
	// everywhere, buddhist and persian where they are the default. Any other
	// is dropped from the locale and the date is in the locale's default
	// calendar, which resolvedOptions().calendar then says.
	ca, caInLocale := r.unicodeKeyword("ca", caOption, func(v string) bool { return calendarFor(r.data, v).name == v })
	nu, nuInLocale := r.unicodeKeyword("nu", nuOption, supportedNumberingSystem)
	hcKeyword := r.keywords["hc"]
	if !contains(hourCycles, hcKeyword) {
		hcKeyword = ""
	}
	if ca == "" {
		ca = defaultCalendar(r.region)
	}
	if nu == "" {
		nu = r.data.str("n/defaultNumberingSystem")
	}
	cal := calendarFor(r.data, ca)
	ns := numberSystemFor(r.data, nu)
	lang := r.tag.lang
	g := generatorFor(cal, lang, r.region, hcKeyword, ns.decimal, true)

	// The hour cycle: the option, else the keyword, else the locale's; hour12
	// picks the locale's 12 or 24 hour flavor.
	hcDefault := g.hourCycle()
	hc := hourCycle
	if hc == "" {
		hc = hcKeyword
	}
	switch {
	case hasHour12 && hour12:
		hc = "h12"
		switch {
		case hcDefault == "h11" || hcDefault == "h12":
			hc = hcDefault
		case r.tag.region == "JP":
			hc = "h11"
		}
	case hasHour12:
		hc = "h23"
		if hcDefault == "h23" || hcDefault == "h24" {
			hc = hcDefault
		}
	case hc == "":
		hc = hcDefault
	}

	f := &dateTimeFormat{
		Calendar:        cal.name,
		NumberingSystem: nu,
		DataLocale:      r.data.name,
		Region:          r.region,
		BaseLocale:      r.locale,
	}
	if f.TimeZone, err = timeZoneOption(o, defaultZone); err != nil {
		return nil, err
	}

	f.ZoneID = zoneNames()[strings.ToUpper(f.TimeZone)]

	var skel strings.Builder
	explicit := map[string]bool{}
	hasHour := false
	for _, c := range components(hc) {
		if c.key == "timeZoneName" {
			n, ok, err := optInt(o, "fractionalSecondDigits", 1, 3)
			if err != nil {
				return nil, err
			}

			if ok {
				explicit["fractionalSecondDigits"] = true
				skel.WriteString(strings.Repeat("S", n))
			}
		}
		v, err := optString(o, c.key, c.allowed...)
		if err != nil {
			return nil, err
		}

		if v != "" {
			explicit[c.key] = true
			hasHour = hasHour || c.key == "hour"
			skel.WriteString(c.letters[v])
		}
	}
	if _, err := optString(o, "formatMatcher", "basic", "best fit"); err != nil {
		return nil, err
	}

	if f.DateStyle, err = optString(o, "dateStyle", styleWidths...); err != nil {
		return nil, err
	}

	if f.TimeStyle, err = optString(o, "timeStyle", styleWidths...); err != nil {
		return nil, err
	}

	dtfHourCycle := ""
	if f.TimeStyle != "" {
		dtfHourCycle = hc
	}
	if f.DateStyle != "" || f.TimeStyle != "" {
		switch {
		case len(explicit) != 0:
			return nil, typeError("Invalid option : option")
		case required == "date" && f.TimeStyle != "":
			return nil, typeError("Invalid option : timeStyle")
		case required == "time" && f.DateStyle != "":
			return nil, typeError("Invalid option : dateStyle")
		}

		_, generated := cldr().supplemental().get("gt/" + r.locale)
		f.Pattern = stylePattern(f.DateStyle, f.TimeStyle, cal, g, lang, r.region, hcKeyword, generated, ns.decimal, dtfHourCycle)
	} else {
		needDefaults := true
		if required == "date" || required == "any" {
			for _, k := range []string{"weekday", "year", "month", "day"} {
				needDefaults = needDefaults && !explicit[k]
			}
		}
		if required == "time" || required == "any" {
			for _, k := range []string{"dayPeriod", "hour", "minute", "second", "fractionalSecondDigits"} {
				needDefaults = needDefaults && !explicit[k]
			}
		}
		if needDefaults && (defaults == "date" || defaults == "all") {
			skel.WriteString("yMd")
		}
		if needDefaults && (defaults == "time" || defaults == "all") {
			switch hc {
			case "h12":
				skel.WriteString("hms")
			case "h11":
				skel.WriteString("Kms")
			case "h24":
				skel.WriteString("kms")
			default:
				skel.WriteString("Hms")
			}
		}
		if hasHour {
			dtfHourCycle = hc
		}
		f.Pattern = replaceHourCycle(g.bestPattern(skel.String()), dtfHourCycle)
	}

	// The reported locale keeps the keywords that were used. An hour12 or
	// hourCycle option drops a -u-hc- that disagrees with the result.
	var keywords [][2]string
	if caInLocale {
		keywords = append(keywords, [2]string{"ca", ca})
	}
	if hcKeyword != "" && (!hasHour12 && hourCycle == "" || dtfHourCycle == hcKeyword) {
		keywords = append(keywords, [2]string{"hc", hcKeyword})
	}
	if nuInLocale {
		keywords = append(keywords, [2]string{"nu", nu})
	}
	f.Locale = r.localeString(keywords)

	if dtfHourCycle != "" {
		h12 := dtfHourCycle == "h11" || dtfHourCycle == "h12"
		f.HourCycle, f.Hour12 = dtfHourCycle, &h12
	}
	if f.DateStyle == "" && f.TimeStyle == "" {
		f.readComponents()
	}
	return f, f.init()
}

// readComponents sets the components resolvedOptions reports from the
// pattern, the way V8 does: by looking for the letters in it.
func (f *dateTimeFormat) readComponents() {
	set := map[string]*string{
		"weekday": &f.Weekday, "era": &f.Era, "year": &f.Year, "month": &f.Month, "day": &f.Day,
		"dayPeriod": &f.DayPeriod, "hour": &f.Hour, "minute": &f.Minute, "second": &f.Second,
		"timeZoneName": &f.TimeZoneName,
	}
	for _, it := range patternItems {
		for _, p := range it.pairs {
			if strings.Contains(f.Pattern, p[0]) {
				*set[it.key] = p[1]
				break
			}
		}
	}
	f.FractionalSecondDigits = min(strings.Count(f.Pattern, "S"), 3)
}

// stylePattern is the pattern of dateStyle and timeStyle, V8's
// DateTimeStylePattern over ICU's SimpleDateFormat::construct. generated
// reports a locale ICU finds its data for under another region or language,
// which has its time pattern generated.
func stylePattern(dateStyle, timeStyle string, cal *calendarData, g *dtpg, lang, region, hcKeyword string, generated bool, decimal, hc string) string {
	if timeStyle == "" {
		return cal.str("dateFormats/" + dateStyle)
	}

	var pattern string
	if hcKeyword != "" || generated {
		// A locale asking for an hour cycle has its time pattern generated,
		// and so does one whose data is another region's or language's.
		pattern = generatorFor(cal, lang, region, hcKeyword, decimal, false).bestPattern(timeSkeletons[styleIndex[timeStyle]])
	}
	if pattern == "" {
		pattern = cal.str("timeFormats/" + timeStyle)
	}
	if dateStyle != "" {
		glue, ok := cal.get("dateTimeFormats-atTime/standard/" + dateStyle)
		if !ok {
			glue = cal.str("dateTimeFormats/" + dateStyle)
		}
		pattern = simpleFormat(glue, pattern, cal.str("dateFormats/"+dateStyle))
	}
	if hc == hourCycleOf(pattern) {
		return pattern
	}

	skel := newSkeleton(pattern).String()
	var b strings.Builder
	for i := 0; i < len(skel); i++ {
		switch c := skel[i]; c {
		case 'a', 'b', 'B':
		case 'h', 'H', 'K', 'k':
			b.WriteByte(hourLetter(hc))
		default:
			b.WriteByte(c)
		}
	}
	return replaceHourCycle(g.bestPattern(b.String()), hc)
}

func hourLetter(hc string) byte {
	switch hc {
	case "h11":
		return 'K'
	case "h12":
		return 'h'
	case "h24":
		return 'k'
	}
	return 'H'
}

// hourCycleOf returns the hour cycle of the first hour field of pattern.
func hourCycleOf(pattern string) string {
	inQuote := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\'':
			inQuote = !inQuote
		case 'K':
			if !inQuote {
				return "h11"
			}
		case 'h':
			if !inQuote {
				return "h12"
			}
		case 'H':
			if !inQuote {
				return "h23"
			}
		case 'k':
			if !inQuote {
				return "h24"
			}
		}
	}
	return ""
}

// replaceHourCycle is V8's ReplaceHourCycleInPattern: every unquoted hour
// letter becomes hc's, with a space put between a day and an hour that
// touch.
func replaceHourCycle(pattern, hc string) string {
	if hc == "" {
		return pattern
	}

	to := hourLetter(hc)
	var b strings.Builder
	replace := true
	var last byte
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '\'':
			replace = !replace
			b.WriteByte(c)
		case 'H', 'h', 'K', 'k':
			if replace && last == 'd' {
				b.WriteByte(' ')
			}
			if replace {
				b.WriteByte(to)
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
		last = c
	}
	return b.String()
}

// wellFormedType reports whether s is a Unicode locale type: one or more
// alphanumeric subtags of 3 to 8 characters.
func wellFormedType(s string) bool {
	for _, p := range strings.Split(s, "-") {
		if len(p) < 3 || len(p) > 8 || !isAlnum(p) {
			return false
		}
	}
	return true
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// timeZoneOption resolves the timeZone option, defaulting to defaultZone.
func timeZoneOption(o map[string]any, defaultZone string) (string, error) {
	v, ok := o["timeZone"]
	if !ok {
		return defaultZone, nil
	}

	s, ok := v.(string)
	if !ok {
		return "", rangeError("invalid time zone specified: %v", v)
	}

	name, _, err := timeZone(s)
	return name, err
}

// init builds the formatter of a resolved DateTimeFormat.
func (f *dateTimeFormat) init() error {
	d := cldr().locale(f.DataLocale)
	if d == nil {
		return rangeError("no locale data %s", f.DataLocale)
	}

	_, loc, err := timeZone(f.TimeZone)
	if err != nil {
		return err
	}

	ns := numberSystemFor(d, f.NumberingSystem)
	digits := ns.digits
	if digits == nil {
		digits = strings.Split("0123456789", "")
	}
	items := parsePattern(f.Pattern)
	df := &dateFormatter{
		items:      items,
		cal:        calendarFor(d, f.Calendar),
		ns:         ns,
		zones:      &zoneNamer{d: d, region: f.Region, digits: digits},
		zoneID:     f.ZoneID,
		loc:        loc,
		dayPeriods: dayPeriodsFor(f.BaseLocale),
	}
	for _, it := range items {
		df.hasMinute = df.hasMinute || it.ch == 'm'
		df.hasSecond = df.hasSecond || it.ch == 's'
	}
	f.f = df
	return nil
}

// format renders the time value ms as ECMA-402 parts.
func (f *dateTimeFormat) format(ms float64) ([]part, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return nil, rangeError("Invalid time value")
	}

	// TimeClip: the integer part, and never -0.
	ms = math.Trunc(ms) + 0
	if f.f == nil {
		if err := f.init(); err != nil {
			return nil, err
		}
	}

	return f.f.format(ms), nil
}
