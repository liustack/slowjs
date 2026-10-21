package intl

import (
	"fmt"
	"sort"
	"strings"
)

// A locale identifier, and the part of it that data is keyed by.
//
// These are two different things and keeping them apart is what lets the rest
// of the library stay honest. A Locale is what the caller asked for, all of
// it: the language, the script, the region, any variants, and the extensions
// that ask for a calendar or a numbering system. A DataLocale is only the part
// a table is stored under. Asking for "de-CH-u-ca-buddhist" loads the data for
// "de-CH" and then formats with a Buddhist calendar; the extension chooses
// behavior, it does not choose a table.
//
// Conflating the two is how a locale ends up meaning "the record loaded for a
// locale", which is what it means in the package this one replaces.

// A DataLocale is the language, script and region a table is stored under. It
// holds no slices, so it is comparable and can be a map key.
//
// Variant is set for one variant alone, "posix": ICU's en_US_POSIX is the
// only data it keeps under a variant, and "-u-va-posix", which ICU turns into
// that variant, is how ECMA-402 asks for it. Its collation, number patterns
// and word breaks differ from en-US's; a table without a POSIX bundle falls
// back to the locale without the variant.
type DataLocale struct {
	Language Language
	Script   Script
	Region   Region
	Variant  Variant
}

// posixVariant is the one variant a data locale holds.
var posixVariant = mustVariant("posix")

func mustVariant(s string) Variant {
	v, err := ParseVariant(s)
	if err != nil {
		panic(err)
	}
	return v
}

// A Keyword is one setting from the Unicode extension: "ca" and "buddhist".
type Keyword struct {
	Key   string
	Value string
}

// An Extension is a singleton extension other than the Unicode one, kept
// whole so that an identifier written out again says what it said.
type Extension struct {
	Singleton byte
	Value     string
}

// A Locale is a locale identifier.
type Locale struct {
	Language Language
	Script   Script
	Region   Region
	Variants []Variant

	// Attributes and Keywords are the Unicode extension, the "-u-" one, which
	// is where a calendar, a numbering system or a collation is asked for.
	// Keywords are held sorted by key so that two identifiers saying the same
	// thing are written the same way.
	Attributes []string
	Keywords   []Keyword

	// Extensions are the other singletons, "-t-" among them, and Private is
	// the "-x-" tail. Neither changes what is loaded; they are kept so that an
	// identifier survives being parsed and written out again.
	Extensions []Extension
	Private    string
}

// Data returns the part of the identifier that data is keyed by: the
// language, script and region, and the variant POSIX where the identifier
// asks for it, by "-u-va-posix" or by the variant itself, as ICU reads both.
func (l Locale) Data() DataLocale {
	d := DataLocale{Language: l.Language, Script: l.Script, Region: l.Region}
	if v, ok := l.Keyword("va"); ok && v == "posix" {
		d.Variant = posixVariant
	}
	if len(l.Variants) == 1 && l.Variants[0] == posixVariant {
		d.Variant = posixVariant
	}
	return d
}

// Keyword returns the value of one Unicode extension setting.
func (l Locale) Keyword(key string) (string, bool) {
	for _, k := range l.Keywords {
		if k.Key == key {
			return k.Value, true
		}
	}
	return "", false
}

// ParseLocale reads a locale identifier. It accepts the forms a tag is written
// in -- "en_GB" as well as "en-GB", in any case -- and answers with the
// canonical one, so that parsing and writing out again settles spelling.
func ParseLocale(s string) (Locale, error) {
	var l Locale
	if strings.TrimSpace(s) == "" {
		return l, fmt.Errorf("%w: the identifier is empty", ErrSyntax)
	}
	parts := strings.Split(strings.ReplaceAll(s, "_", "-"), "-")
	for _, p := range parts {
		if p == "" {
			return l, fmt.Errorf("%w: %q has an empty subtag", ErrSyntax, s)
		}
	}

	at := 0
	var err error
	if l.Language, err = ParseLanguage(parts[0]); err != nil {
		return Locale{}, err
	}
	at++

	// Script, then region, then variants, each optional and each recognized by
	// its shape. A subtag of one character is a singleton and ends the
	// language identifier.
	if at < len(parts) && len(parts[at]) == 4 && alphaOnly(parts[at]) {
		if l.Script, err = ParseScript(parts[at]); err != nil {
			return Locale{}, err
		}
		at++
	}
	if at < len(parts) && isRegionShaped(parts[at]) {
		if l.Region, err = ParseRegion(parts[at]); err != nil {
			return Locale{}, err
		}
		at++
	}
	for at < len(parts) && len(parts[at]) != 1 {
		v, err := ParseVariant(parts[at])
		if err != nil {
			return Locale{}, err
		}
		l.Variants = append(l.Variants, v)
		at++
	}

	if err := l.parseExtensions(parts[at:], s); err != nil {
		return Locale{}, err
	}
	l.canonicalize()
	return l, nil
}

// isRegionShaped reports whether a subtag is in the region position's shape,
// which is what tells a region from the variant that may follow it.
func isRegionShaped(s string) bool {
	return len(s) == 2 && alphaOnly(s) || len(s) == 3 && digitsOnly(s)
}

// parseExtensions reads the singleton extensions that follow the language
// identifier. Each runs until the next singleton or the end.
func (l *Locale) parseExtensions(parts []string, whole string) error {
	seen := map[byte]bool{}
	for at := 0; at < len(parts); {
		if len(parts[at]) != 1 {
			return fmt.Errorf("%w: %q has %q where an extension was expected",
				ErrSyntax, whole, parts[at])
		}
		singleton := toLower(parts[at][0])
		if !isAlpha(singleton) && !isDigit(singleton) {
			return fmt.Errorf("%w: %q is not an extension", ErrSyntax, parts[at])
		}
		if seen[singleton] {
			return fmt.Errorf("%w: %q gives -%c- twice", ErrSyntax, whole, singleton)
		}
		seen[singleton] = true
		at++

		start := at
		// Private use runs to the end; every other extension stops at the next
		// singleton.
		for at < len(parts) && (singleton == 'x' || len(parts[at]) != 1) {
			at++
		}
		body := parts[start:at]
		if len(body) == 0 {
			return fmt.Errorf("%w: %q gives -%c- with nothing after it",
				ErrSyntax, whole, singleton)
		}
		for _, b := range body {
			if !alphanumOnly(b) || len(b) > 8 {
				return fmt.Errorf("%w: %q in -%c-", ErrSyntax, b, singleton)
			}
		}
		switch singleton {
		case 'u':
			l.parseUnicode(body)
		case 'x':
			l.Private = strings.ToLower(strings.Join(body, "-"))
		default:
			l.Extensions = append(l.Extensions, Extension{
				Singleton: singleton,
				Value:     strings.ToLower(strings.Join(body, "-")),
			})
		}
	}
	return nil
}

// parseUnicode reads the Unicode extension: any attributes first, then
// keywords, each a two-character key and the subtags that are its value.
func (l *Locale) parseUnicode(body []string) {
	at := 0
	for at < len(body) && len(body[at]) != 2 {
		l.Attributes = append(l.Attributes, strings.ToLower(body[at]))
		at++
	}
	for at < len(body) {
		key := strings.ToLower(body[at])
		at++
		start := at
		for at < len(body) && len(body[at]) != 2 {
			at++
		}
		value := strings.ToLower(strings.Join(body[start:at], "-"))
		// A keyword whose value is "true" is written without one, so that
		// "-u-kn" and "-u-kn-true" are one identifier rather than two.
		if value == "true" {
			value = ""
		}
		l.Keywords = append(l.Keywords, Keyword{Key: key, Value: value})
	}
}

// canonicalize puts the parts that have no order of their own into one, so
// that two identifiers meaning the same thing are written the same way.
//
// The sorts are stable: of a key given twice, the first is the one meant,
// and a sort that is not stable, as sort.Slice is past twelve elements,
// could put the second first.
func (l *Locale) canonicalize() {
	sort.SliceStable(l.Variants, func(i, j int) bool {
		return l.Variants[i].String() < l.Variants[j].String()
	})
	sort.Strings(l.Attributes)
	sort.SliceStable(l.Keywords, func(i, j int) bool { return l.Keywords[i].Key < l.Keywords[j].Key })
	sort.SliceStable(l.Extensions, func(i, j int) bool {
		return l.Extensions[i].Singleton < l.Extensions[j].Singleton
	})
}

// String writes the identifier in its canonical form.
func (l Locale) String() string {
	var b strings.Builder
	b.WriteString(l.Language.String())
	if !l.Script.IsZero() {
		b.WriteByte('-')
		b.WriteString(l.Script.String())
	}
	if !l.Region.IsZero() {
		b.WriteByte('-')
		b.WriteString(l.Region.String())
	}
	for _, v := range l.Variants {
		b.WriteByte('-')
		b.WriteString(v.String())
	}
	// The extensions in the order of their singletons, the Unicode one
	// among them.
	unicode := len(l.Attributes) > 0 || len(l.Keywords) > 0
	for _, e := range l.Extensions {
		if unicode && e.Singleton > 'u' {
			l.writeUnicode(&b)
			unicode = false
		}
		b.WriteByte('-')
		b.WriteByte(e.Singleton)
		b.WriteByte('-')
		b.WriteString(e.Value)
	}
	if unicode {
		l.writeUnicode(&b)
	}
	if l.Private != "" {
		b.WriteString("-x-")
		b.WriteString(l.Private)
	}
	return b.String()
}

// writeUnicode writes the Unicode extension.
func (l Locale) writeUnicode(b *strings.Builder) {
	b.WriteString("-u")
	for _, a := range l.Attributes {
		b.WriteByte('-')
		b.WriteString(a)
	}
	for _, k := range l.Keywords {
		b.WriteByte('-')
		b.WriteString(k.Key)
		if k.Value != "" {
			b.WriteByte('-')
			b.WriteString(k.Value)
		}
	}
}

// String writes the data locale, which is an identifier with nothing but the
// language, script and region, and the variant where there is one.
func (d DataLocale) String() string {
	out := d.Language.String()
	if !d.Script.IsZero() {
		out += "-" + d.Script.String()
	}
	if !d.Region.IsZero() {
		out += "-" + d.Region.String()
	}
	if !d.Variant.IsZero() {
		out += "-" + d.Variant.String()
	}
	return out
}

// IsRoot reports whether the data locale names no language, script or region,
// which is the root the fallback chain ends at.
func (d DataLocale) IsRoot() bool { return d == DataLocale{} }

// DataLocaleSize is how many bytes a data locale takes when written out: its
// three subtags, each in its own fixed-width field. The tables written so --
// likely subtags, parent locales -- know no variants, and a variant is not
// written.
//
// Generated tables are records of this size laid end to end and sorted, so a
// lookup is a binary search over the bytes with nothing decoded on the way.
// The generator and the reader share this one definition rather than each
// spelling the layout out, so the two cannot drift.
const DataLocaleSize = len(Language{}) + len(Script{}) + len(Region{})

// MarshalBinary writes the data locale as DataLocaleSize bytes.
func (d DataLocale) MarshalBinary() ([]byte, error) {
	return d.AppendBinary(make([]byte, 0, DataLocaleSize))
}

// AppendBinary appends the data locale to b.
func (d DataLocale) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, d.Language[:]...)
	b = append(b, d.Script[:]...)
	b = append(b, d.Region[:]...)
	return b, nil
}

// UnmarshalBinary reads a data locale written by AppendBinary.
func (d *DataLocale) UnmarshalBinary(b []byte) error {
	if len(b) != DataLocaleSize {
		return fmt.Errorf("a data locale is %d bytes, not %d", DataLocaleSize, len(b))
	}
	var out DataLocale
	n := copy(out.Language[:], b)
	n += copy(out.Script[:], b[n:])
	copy(out.Region[:], b[n:])
	*d = out
	return nil
}
