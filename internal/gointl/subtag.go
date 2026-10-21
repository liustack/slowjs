package intl

import (
	"errors"
	"fmt"
)

// The pieces a locale identifier is made of.
//
// Each is held as its letters in a fixed-size array rather than as a string:
// a subtag is at most eight characters, so the array is no larger than the
// string header would be, it needs no allocation, and -- the reason that
// matters -- it is comparable, so a locale built out of these can be a map key
// and compared with ==. Unused positions are zero.
//
// The letters are stored already in their canonical case, which is the case
// UTS #35 writes them in: a language lower, a script with one capital, a
// region upper. Two identifiers that name the same locale therefore hold the
// same bytes, and comparing them needs no folding.

// ErrSyntax reports an identifier or subtag that is not well formed. ECMA-402
// answers a tag like this with a RangeError.
var ErrSyntax = errors.New("not a well-formed locale identifier")

// A Language is a language subtag: two or three letters, or five to eight.
// The zero Language is "und", the undetermined language.
type Language [8]byte

// A Script is a script subtag: exactly four letters, written Titlecase.
type Script [4]byte

// A Region is a region subtag: two letters, written uppercase, or three
// digits.
type Region [3]byte

// A Variant is a variant subtag: five to eight letters or digits, or four
// beginning with a digit.
type Variant [8]byte

// ParseLanguage reads a language subtag.
func ParseLanguage(s string) (Language, error) {
	var l Language
	switch {
	case s == "" || equalFold(s, "root"):
		return Language{}, nil
	case !alphaOnly(s):
		return l, fmt.Errorf("%w: language %q is not letters", ErrSyntax, s)
	case len(s) < 2 || len(s) == 4 || len(s) > 8:
		// Four letters is a script's length, and the grammar keeps the
		// position free so that one is never read as the other.
		return l, fmt.Errorf("%w: language %q is %d letters", ErrSyntax, s, len(s))
	}
	for i := 0; i < len(s); i++ {
		l[i] = toLower(s[i])
	}
	if l == undBytes {
		return Language{}, nil
	}
	return l, nil
}

var undBytes = Language{'u', 'n', 'd'}

// ParseScript reads a script subtag.
func ParseScript(s string) (Script, error) {
	var sc Script
	if s == "" {
		return sc, nil
	}
	if len(s) != 4 || !alphaOnly(s) {
		return sc, fmt.Errorf("%w: script %q is not four letters", ErrSyntax, s)
	}
	sc[0] = toUpper(s[0])
	for i := 1; i < 4; i++ {
		sc[i] = toLower(s[i])
	}
	return sc, nil
}

// ParseRegion reads a region subtag.
func ParseRegion(s string) (Region, error) {
	var r Region
	switch {
	case s == "":
		return r, nil
	case len(s) == 2 && alphaOnly(s):
		r[0], r[1] = toUpper(s[0]), toUpper(s[1])
	case len(s) == 3 && digitsOnly(s):
		r[0], r[1], r[2] = s[0], s[1], s[2]
	default:
		return r, fmt.Errorf("%w: region %q is not two letters or three digits",
			ErrSyntax, s)
	}
	return r, nil
}

// ParseVariant reads a variant subtag.
func ParseVariant(s string) (Variant, error) {
	var v Variant
	ok := (len(s) >= 5 && len(s) <= 8 && alphanumOnly(s)) ||
		(len(s) == 4 && isDigit(s[0]) && alphanumOnly(s))
	if !ok {
		return v, fmt.Errorf("%w: variant %q", ErrSyntax, s)
	}
	for i := 0; i < len(s); i++ {
		v[i] = toLower(s[i])
	}
	return v, nil
}

// String writes the subtag. The undetermined language is written "und", which
// is what it is called wherever a language has to be named.
func (l Language) String() string {
	if l.IsUnd() {
		return "und"
	}
	return trimZero(l[:])
}

// IsUnd reports whether the language is the undetermined one, "und", which
// is what an identifier that names no language means and what "root" is
// written as. It is the zero Language.
func (l Language) IsUnd() bool { return l == Language{} }

func (s Script) String() string  { return trimZero(s[:]) }
func (r Region) String() string  { return trimZero(r[:]) }
func (v Variant) String() string { return trimZero(v[:]) }

// IsZero reports whether the subtag is absent. A language is never absent --
// it is "und" instead -- so Language has no IsZero.
func (s Script) IsZero() bool  { return s == Script{} }
func (r Region) IsZero() bool  { return r == Region{} }
func (v Variant) IsZero() bool { return v == Variant{} }

// trimZero reads the letters out of a fixed-size subtag.
func trimZero(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// The character tests are written out rather than taken from the unicode
// package: a subtag is ASCII by definition, and a tag that carries anything
// else is not well formed rather than in need of folding.

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func toUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func alphaOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlpha(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

func digitsOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

func alphanumOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlpha(s[i]) && !isDigit(s[i]) {
			return false
		}
	}
	return len(s) > 0
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if toLower(a[i]) != toLower(b[i]) {
			return false
		}
	}
	return true
}
