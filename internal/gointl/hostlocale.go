package intl

import "strings"

// HostLocale is the locale the machine is set to, as ICU's default locale
// finds it (uprv_getDefaultLocaleID). On Windows it is the user's locale;
// elsewhere it is LC_ALL, LC_MESSAGES or LANG, its codeset dropped, "C" and
// "POSIX" being en_US_POSIX, "en-US-u-va-posix". What cannot be read is
// "en-US".
//
// V8 takes ICU's default for Intl's default locale but for that fallback,
// which it makes "en-US" (Isolate::DefaultLocale); an engine that answers
// as Node does does the same.
func HostLocale() Locale {
	if loc, ok := localeFromICUID(hostLocaleID()); ok {
		return loc
	}
	loc, _ := ParseLocale("en-US")
	return loc
}

// localeFromICUID reads an ICU locale ID, "de_DE" or "en_US_POSIX", as
// ICU's toLanguageTag writes it: the language, script and region, then the
// variants, a POSIX one becoming "-u-va-posix", and from the first that is
// not a BCP 47 variant on, private use after "lvariant". So the POSIX
// locale "de_DE.UTF-8@euro", ID "de_DE_euro", is "de-DE-x-lvariant-euro",
// and "de@euro" "de-x-lvariant-euro", not the script Euro. Such a tag is
// read but not canonicalized, which refuses it, as Node refuses it as a
// requested locale.
func localeFromICUID(id string) (Locale, bool) {
	if id == "" {
		return Locale{}, true
	}
	parts := strings.Split(id, "_")
	subtags := []string{strings.ToLower(parts[0])}
	rest := parts[1:]
	if len(rest) > 0 && len(rest[0]) == 4 && isLetters(rest[0]) {
		subtags = append(subtags, rest[0])
		rest = rest[1:]
	}
	if len(rest) > 0 {
		switch r := rest[0]; {
		case len(r) == 2 && isLetters(r), len(r) == 3 && isDigits(r):
			subtags = append(subtags, r)
			rest = rest[1:]
		case r == "":
			// "ab__CD" is a language and a variant.
			rest = rest[1:]
		}
	}
	var private []string
	for _, v := range rest {
		if v == "" {
			continue
		}
		v = strings.ToLower(v)
		switch _, err := ParseVariant(v); {
		case err == nil && private == nil:
			subtags = append(subtags, v)
		case len(v) <= 8 && isAlphanumeric(v):
			private = append(private, v)
		}
		// A subtag private use cannot hold either, longer than eight, ICU
		// leaves out: "es_ES_TRADITIONAL" is "es-ES".
	}
	tag := strings.Join(subtags, "-")
	if private != nil {
		tag += "-x-lvariant-" + strings.Join(private, "-")
	}
	if c, err := NewCanonicalizer(Embedded, CanonicalizeOptions{}); err == nil {
		if loc, err := c.Canonicalize(tag); err == nil {
			return loc, true
		}
	}
	loc, err := ParseLocale(tag)
	return loc, err == nil
}

// isAlphanumeric reports whether s is all ASCII letters and digits.
func isAlphanumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlpha(s[i]) && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return s != ""
}

// isDigits reports whether s is all ASCII digits.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
