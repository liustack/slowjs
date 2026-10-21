package intl

import (
	"strconv"
	"strings"
)

// Weeks: which day a week starts on and how many days of a new year its first
// week needs, as CLDR gives them by region. The week-based year of a date
// ("Y") depends on both: the last days of December may fall in the first
// week of the next year.

// weekRules are one region's week conventions.
type weekRules struct {
	firstDay int // Sunday is zero
	minDays  int
}

// loadWeekRules reads the week conventions of a locale's region, or of the
// region it most likely means, falling back to the world's.
func loadWeekRules(src Source, loc Locale) weekRules {
	rules := weekRules{firstDay: 1, minDays: 1}
	b, err := src.Open(MarkerWeekData, DataLocale{})
	if err != nil {
		return rules
	}
	region := regionForSupplementalData(src, loc)
	if region == "" {
		if f, err := NewFallbacker(src); err == nil {
			if full, ok := f.Maximize(loc.Data()); ok {
				region = full.Region.String()
			}
		}
	}
	values := parseWeekData(b)
	pick := func(field string, into *int) {
		if v, ok := values[field][region]; ok {
			*into = v
		} else if v, ok := values[field]["001"]; ok {
			*into = v
		}
	}
	pick("firstDay", &rules.firstDay)
	pick("minDays", &rules.minDays)
	return rules
}

// parseWeekData reads data/weekdata.bin: by field, "firstDay", "minDays",
// "weekendStart" and "weekendEnd", the value for each region, days counted
// from Sunday as zero.
func parseWeekData(b []byte) map[string]map[string]int {
	values := map[string]map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		v, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		if values[fields[0]] == nil {
			values[fields[0]] = map[string]int{}
		}
		values[fields[0]][fields[1]] = v
	}
	return values
}

// weekYear is the year of the week a date falls in, as ICU's
// Calendar::computeWeekFields reckons it from the extended year.
func (f *DateTimeFormat) weekYear(p *dateParts) int {
	year := p.extYear
	if p.yearLength == nil {
		return year
	}
	first, minDays := f.week.firstDay, f.week.minDays
	relDow := mod(p.weekday-first, 7)
	relDowJan1 := mod(p.weekday-(p.dayOfYear-1)-first, 7)
	woy := (p.dayOfYear - 1 + relDowJan1) / 7
	if 7-relDowJan1 >= minDays {
		woy++
	}
	if woy == 0 {
		return year - 1
	}
	last := p.yearLength(year)
	if p.dayOfYear >= last-5 {
		lastRelDow := mod(relDow+last-p.dayOfYear, 7)
		if 6-lastRelDow >= minDays && p.dayOfYear+7-relDow > last {
			return year + 1
		}
	}
	return year
}

// mod is the remainder that is never negative.
func mod(a, b int) int {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}
