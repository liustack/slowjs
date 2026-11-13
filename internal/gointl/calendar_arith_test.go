//go:build gointl_all

package intl_test

import (
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The arithmetic calendars across their era boundaries, where the sweep's
// modern dates do not reach: before Diocletian, the Incarnation and the
// Republic of China, and far before the Hijra. The expectations are Node's;
// English has no name for the Coptic era before Diocletian, so none is
// written.
func TestArithmeticCalendars(t *testing.T) {
	dates := []time.Time{
		time.Date(200, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1905, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1911, 12, 31, 0, 0, 0, 0, time.UTC),
		time.Date(1912, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(-600, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for calendar, want := range map[string][]string{
		"coptic":        {"10/7/85 ", "4/23/1621 AM", "4/21/1628 AM", "4/22/1628 AM", "5/12/885 "},
		"ethiopic":      {"10/7/192 AM", "4/23/1897 AM", "4/21/1904 AM", "4/22/1904 AM", "5/12/4892 AA"},
		"ethioaa":       {"10/7/5692 AA", "4/23/7397 AA", "4/21/7404 AA", "4/22/7404 AA", "5/12/4892 AA"},
		"indian":        {"3/11/122 Śaka", "10/11/1826 Śaka", "10/10/1833 Śaka", "10/11/1833 Śaka", "10/11/-679 Śaka"},
		"islamic-civil": {"11/30/-435 AH", "10/24/1322 AH", "1/10/1330 AH", "1/11/1330 AH", "12/7/-1260 AH"},
		"islamic-tbla":  {"12/1/-435 AH", "10/25/1322 AH", "1/11/1330 AH", "1/12/1330 AH", "12/8/-1260 AH"},
		"roc":           {"6/1/1712 B.R.O.C.", "1/1/7 B.R.O.C.", "12/31/1 B.R.O.C.", "1/1/1 Minguo", "1/8/2512 B.R.O.C."},
	} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{
			Calendar: calendar, TimeZone: "UTC", Era: intl.WidthShort,
			Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
			Compat: intl.NodeICU,
		})
		for i, d := range dates {
			if got := f.Format(d); got != want[i] {
				t.Errorf("%s %s = %q, want %q", calendar, d.Format("2006-01-02"), got, want[i])
			}
		}
	}
}

// The Hebrew calendar names its leap month as ICU does: Adar I and Adar II
// in a leap year, plain Adar in a common one, and Hebrew writes its dates in
// Hebrew numerals, the year without its thousands. The expectations are
// Node's.
func TestHebrewCalendar(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 2, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 4, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 4, 20, 0, 0, 0, 0, time.UTC),
	}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want []string
	}{
		{"en-u-ca-hebrew", intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
			Month: intl.WidthLong, Day: intl.WidthNumeric},
			[]string{"11 Adar I 5784", "10 Adar II 5784", "12 Nisan 5784", "20 Adar 5785", "22 Nisan 5785"}},
		{"he-u-ca-hebrew", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthLong},
			[]string{"י״א באדר א׳ תשפ״ד", "י׳ באדר ב׳ תשפ״ד", "י״ב בניסן תשפ״ד", "כ׳ באדר תשפ״ה", "כ״ב בניסן תשפ״ה"}},
	} {
		f := newDateTime(t, c.tag, c.opts)
		for i, d := range dates {
			if got := f.Format(d); got != c.want[i] {
				t.Errorf("%s %s = %q, want %q", c.tag, d.Format("2006-01-02"), got, c.want[i])
			}
		}
	}
}

// The Japanese calendar changes era on the day an era starts, calls an
// era's first year 元年 in Japanese, keeps ICU's Julian dates before 1582,
// and before the first era counts back through it. The expectations are
// Node's.
func TestJapaneseCalendar(t *testing.T) {
	dates := []time.Time{
		time.Date(2019, 5, 30, 0, 0, 0, 0, time.UTC),
		time.Date(1989, 1, 7, 0, 0, 0, 0, time.UTC),
		time.Date(1989, 1, 8, 0, 0, 0, 0, time.UTC),
		time.Date(1912, 7, 29, 0, 0, 0, 0, time.UTC),
		time.Date(1912, 7, 30, 0, 0, 0, 0, time.UTC),
		time.Date(1868, 10, 23, 0, 0, 0, 0, time.UTC),
		time.Date(600, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1500, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want []string
	}{
		{"en-u-ca-japanese", intl.DateTimeFormatOptions{TimeZone: "UTC", Era: intl.WidthLong,
			Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric},
			[]string{"5/30/1 Reiwa", "1/7/64 Shōwa", "1/8/1 Heisei", "7/29/45 Meiji", "7/30/1 Taishō",
				"10/23/1 Meiji", "12/30/-45 Taika (645–650)", "12/23/8 Meiō (1492–1501)"}},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthLong},
			[]string{"令和元年5月30日", "昭和64年1月7日", "平成元年1月8日", "明治45年7月29日", "大正元年7月30日",
				"明治元年10月23日", "大化-45年12月30日", "明応8年12月23日"}},
	} {
		f := newDateTime(t, c.tag, c.opts)
		for i, d := range dates {
			if got := f.Format(d); got != c.want[i] {
				t.Errorf("%s %s = %q, want %q", c.tag, d.Format("2006-01-02"), got, c.want[i])
			}
		}
	}
}

// TestISO8601Calendar pins the ISO 8601 calendar to Node: the root's own
// patterns over the locale's names, and the era names ICU 78 is left with
// for want of era rules -- none, but the narrow one before the common era.
func TestISO8601Calendar(t *testing.T) {
	dates := []time.Time{
		time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
		time.Date(-5, 1, 5, 0, 0, 0, 0, time.UTC),
	}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want []string
	}{
		{"en-u-ca-iso8601", intl.DateTimeFormatOptions{TimeZone: "UTC", Era: intl.WidthLong, Year: intl.WidthNumeric},
			[]string{" 2024", " 6"}},
		{"en-u-ca-iso8601", intl.DateTimeFormatOptions{TimeZone: "UTC", Era: intl.WidthNarrow, Year: intl.WidthNumeric},
			[]string{" 2024", "B 6"}},
		{"en-u-ca-iso8601", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthFull},
			[]string{"2024 January 5, Friday", "6 January 5, Thursday"}},
		{"en-u-ca-iso8601", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthShort},
			[]string{"2024-01-05", "6-01-05"}},
		{"de-u-ca-iso8601", intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthMedium, TimeStyle: intl.LengthShort},
			[]string{"2024 Jan. 5, 00:00", "6 Jan. 5, 00:00"}},
	} {
		f := newDateTime(t, c.tag, c.opts)
		for i, d := range dates {
			if got := f.Format(d); got != c.want[i] {
				t.Errorf("%s %v = %q, want %q", c.tag, d.Format("2006-01-02"), got, c.want[i])
			}
		}
	}
}

// TestChineseCalendars pins the Chinese and Korean calendars to Node: the
// leap second month of 2023 in each language's leap-month pattern, the
// cyclic year names, the new year, and Chinese days in hanidays numerals.
func TestChineseCalendars(t *testing.T) {
	leap := time.Date(2023, 3, 25, 0, 0, 0, 0, time.UTC)
	newYear := time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC)
	full := intl.DateTimeFormatOptions{TimeZone: "UTC", DateStyle: intl.LengthFull}
	for _, c := range []struct {
		tag  string
		when time.Time
		want string
	}{
		{"zh-u-ca-chinese", leap, "2023癸卯年闰二月初四星期六"},
		{"en-u-ca-chinese", leap, "Saturday, Second Monthbis 4, 2023(gui-mao)"},
		{"ko-u-ca-dangi", leap, "계묘년 윤2월 4일 토요일"},
		{"zh-u-ca-chinese", newYear, "2024甲辰年正月初一星期六"},
	} {
		if got := newDateTime(t, c.tag, full).Format(c.when); got != c.want {
			t.Errorf("%s %s = %q, want %q", c.tag, c.when.Format("2006-01-02"), got, c.want)
		}
	}

	f := newDateTime(t, "ko-u-ca-dangi", intl.DateTimeFormatOptions{TimeZone: "UTC",
		Year: intl.WidthNumeric, Month: intl.WidthLong, Day: intl.WidthNumeric})
	var got []string
	for _, p := range f.FormatToParts(leap) {
		got = append(got, string(p.Kind)+"="+p.Value)
	}
	want := []string{"relatedYear=2023", "literal=년(", "yearName=계묘", "literal=년) ",
		"month=윤2월", "literal= ", "day=4", "literal=일"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("ko-u-ca-dangi parts = %q, want %q", got, want)
	}
}
