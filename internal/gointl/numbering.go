package intl

import (
	"fmt"
	"sort"

	"github.com/liustack/slowjs/internal/gointl/internal/numdata"
)

// Numbering systems: which ten digits a number is written with.
//
// Every locale has a default -- Latin digits in English, Arabic-Indic in
// Egyptian Arabic, Persian ones in Persian -- and any locale may be asked for
// any of CLDR's numeric systems, with the -u-nu keyword or the
// numberingSystem option. A system the locale's data covers brings its own
// separators; one it does not takes the root's entry, which for most systems
// means the locale's Latin separators around the new digits.

// NumberingSystems returns the names of the numeric numbering systems, sorted:
// what Intl.supportedValuesOf("numberingSystem") lists.
func NumberingSystems() ([]string, error) { return NumberingSystemsFrom(Embedded) }

// NumberingSystemsFrom is NumberingSystems for a source of the caller's own.
func NumberingSystemsFrom(src Source) ([]string, error) {
	systems, err := loadNumberingSystems(src)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(systems))
	for i, s := range systems {
		out[i] = s.Name
	}
	return out, nil
}

func loadNumberingSystems(src Source) ([]numdata.NumberingSystem, error) {
	b, err := src.Open(MarkerNumberingSystems, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the numbering systems: %w", err)
	}
	systems, err := numdata.DecodeSystems(b)
	if err != nil {
		return nil, fmt.Errorf("intl: the numbering systems: %w", err)
	}
	if !sort.SliceIsSorted(systems, func(i, j int) bool { return systems[i].Name < systems[j].Name }) {
		return nil, fmt.Errorf("intl: the numbering systems are not sorted")
	}
	return systems, nil
}

// selectNumberingSystem chooses the numbering system a formatter writes in,
// as ECMA-402's ResolveLocale chooses the "nu" keyword: the option if it names
// a numeric system, else the -u-nu keyword if it does, else the locale's
// default. It returns the number data as written in that system, and the
// keyword value the resolved locale keeps, which is empty unless the keyword
// was honoured.
func selectNumberingSystem(src Source, data *numdata.Locale, loc Locale, option string) (*numdata.Locale, string, error) {
	keyword, _ := loc.keywordValue("nu")
	if option == "" && keyword == "" {
		return data, "", nil
	}
	systems, err := loadNumberingSystems(src)
	if err != nil {
		return nil, "", err
	}
	if option != "" {
		if chosen, ok := data.Select(option, systems); ok {
			if option == keyword {
				return chosen, keyword, nil
			}
			return chosen, "", nil
		}
	}
	if keyword != "" {
		if chosen, ok := data.Select(keyword, systems); ok {
			return chosen, keyword, nil
		}
	}
	return data, "", nil
}

// withKeyword returns the locale with one Unicode extension keyword set to a
// value, or removed when the value is empty.
func (l Locale) withKeyword(key, value string) Locale {
	keep := map[string]string{}
	for _, k := range l.Keywords {
		if k.Key != key {
			keep[k.Key] = k.Value
		}
	}
	if value != "" {
		keep[key] = value
	}
	out := l.withKeywords(keep)
	out.Attributes = l.Attributes
	return out
}

// onlyKeywords returns the locale with its Unicode extension cut down to the
// keys a service uses, which is what ECMA-402's ResolveLocale leaves in a
// resolved locale.
func (l Locale) onlyKeywords(keys ...string) Locale {
	keep := map[string]string{}
	for _, k := range l.Keywords {
		for _, key := range keys {
			if k.Key == key {
				keep[k.Key] = k.Value
			}
		}
	}
	return l.withKeywords(keep)
}
