package intl

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/liustack/quickjs/internal/gointl/internal/listdata"
)

// Intl.ListFormat.
//
// Joining a list is not a matter of putting a separator between the items.
// English writes "a and b" for two and "a, b, and c" for three, which is not
// the same join repeated, and other languages differ again. CLDR gives four
// patterns for each kind of list and this applies them.

// ListType is what kind of list is being written.
type ListType int

const (
	// Conjunction is "a, b, and c", and the default.
	Conjunction ListType = iota
	// Disjunction is "a, b, or c".
	Disjunction
	// UnitList is "a, b, c", for a run of measurements.
	UnitList
)

// ListStyle is how much room the joins take.
type ListStyle int

const (
	// ListLong is the default.
	ListLong ListStyle = iota
	ListShort
	ListNarrow
)

// ListFormatOptions mirrors the option bag of Intl.ListFormat. Its zero value
// is ECMA-402's default in every field.
type ListFormatOptions struct {
	Type  ListType
	Style ListStyle

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A ListFormat joins lists in one locale. It never changes after it is built
// and is safe for any number of goroutines to share.
type ListFormat struct {
	locale   Locale
	opts     ListFormatOptions
	patterns listdata.Patterns
	// context is the language's other two and end patterns, chosen by the
	// item that follows the join; nil for a language without them.
	context *listContext
}

// listContext is a pair of patterns a language uses in place of its own
// before some words, and the test for those words.
type listContext struct {
	test     func(next string) bool
	two, end string
}

// contextualPatterns is ICU's createPatternHandler (listformatter.cpp),
// which changes the join in code, not data: Spanish "y" is "e" before an
// "i" sound and "o" is "u" before an "o" sound, and Hebrew "ו" takes a dash
// before a word not written in Hebrew. Only a pattern that is exactly the
// one ICU looks for changes.
func contextualPatterns(src Source, language string, p listdata.Patterns) (*listContext, error) {
	type rule struct {
		from, to string
		test     func(string) bool
	}
	var rules []rule
	switch language {
	case "es":
		rules = []rule{{"{0} y {1}", "{0} e {1}", spanishE}, {"{0} o {1}", "{0} u {1}", spanishU}}
	case "he", "iw":
		props, err := loadUnicodeProps(src)
		if err != nil {
			return nil, err
		}
		rules = []rule{{"{0} \u05D5{1}", "{0} \u05D5-{1}", func(next string) bool { return hebrewVavDash(props, next) }}}
	}
	for _, r := range rules {
		if p.Two != r.from && p.End != r.from {
			continue
		}
		c := &listContext{test: r.test, two: p.Two, end: p.End}
		if p.Two == r.from {
			c.two = r.to
		}
		if p.End == r.from {
			c.end = r.to
		}
		return c, nil
	}
	return nil, nil
}

// spanishE is ICU's shouldChangeToE: a word that begins "i" or "hi", but not
// "hia" or "hie", whatever the case.
func spanishE(next string) bool {
	lower := func(i int) byte { return next[i] | 0x20 }
	switch {
	case next == "":
		return false
	case lower(0) == 'i':
		return true
	case lower(0) == 'h' && len(next) > 1 && lower(1) == 'i':
		return len(next) == 2 || (lower(2) != 'a' && lower(2) != 'e')
	}
	return false
}

// spanishU is ICU's shouldChangeToU: a word that begins "o", "ho" or "8",
// whatever the case, or the number eleven on its own.
func spanishU(next string) bool {
	switch {
	case next == "":
		return false
	case next[0]|0x20 == 'o', next[0] == '8':
		return true
	case next[0]|0x20 == 'h' && len(next) > 1 && next[1]|0x20 == 'o':
		return true
	}
	return strings.HasPrefix(next, "11") && (len(next) == 2 || next[2] == ' ')
}

// hebrewVavDash is ICU's shouldChangeToVavDash: a word whose first letter
// is not of the Hebrew script.
func hebrewVavDash(props *unicodeProps, next string) bool {
	if next == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(next)
	return !props.in("sc Hebrew", r)
}

// two is the pattern that joins a list of two, before its second item.
func (f *ListFormat) two(next string) string {
	if f.context != nil && f.context.test(next) {
		return f.context.two
	}
	return f.patterns.Two
}

// end is the pattern that joins the last item of a longer list.
func (f *ListFormat) end(last string) string {
	if f.context != nil && f.context.test(last) {
		return f.context.end
	}
	return f.patterns.End
}

// NewListFormat builds a formatter from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewListFormat(loc Locale, opts ListFormatOptions) (*ListFormat, error) {
	return NewListFormatFrom(Embedded, loc, opts)
}

// NewListFormatFrom builds a formatter from a source of the caller's own.
func NewListFormatFrom(src Source, loc Locale, opts ListFormatOptions) (*ListFormat, error) {
	data, err := loadLists(src, loc)
	if err != nil {
		return nil, err
	}
	kind := listdata.Standard
	switch opts.Type {
	case Disjunction:
		kind = listdata.Or
	case UnitList:
		kind = listdata.Unit
	}
	width := listdata.Long
	switch opts.Style {
	case ListShort:
		width = listdata.Short
	case ListNarrow:
		width = listdata.Narrow
	}
	p := data.Get(kind, width)
	if p.Empty() {
		return nil, fmt.Errorf("intl: %s has no list patterns: %w", loc, ErrNotFound)
	}
	context, err := contextualPatterns(src, loc.Language.String(), p)
	if err != nil {
		return nil, err
	}
	// ListFormat uses nothing of the Unicode extension.
	return &ListFormat{locale: loc.onlyKeywords(), opts: opts, patterns: p, context: context}, nil
}

func loadLists(src Source, loc Locale) (*listdata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(treeLocales, loc.Data())
	}
	for _, d := range chain {
		b, err := src.Open(MarkerLists, d)
		if err != nil {
			continue
		}
		l, err := listdata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the list patterns for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no list patterns for %s: %w", loc, ErrNotFound)
}

// Format joins a list.
func (f *ListFormat) Format(items []string) string {
	var b strings.Builder
	for _, p := range f.FormatToParts(items) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// ListPartKind says whether a piece of a joined list is one of the items or
// the text between them.
type ListPartKind string

const (
	ListElement ListPartKind = "element"
	ListLiteral ListPartKind = "literal"
)

// A ListPart is one piece of a joined list.
type ListPart struct {
	Kind  ListPartKind
	Value string
}

// FormatToParts joins a list and says which pieces are the items.
//
// The list is built from the end: the last two items are joined with the end
// pattern, each earlier item is folded in with the middle pattern, and the
// first with the start pattern. That is what makes "a, b, and c" rather than
// one separator repeated.
func (f *ListFormat) FormatToParts(items []string) []ListPart {
	parts := f.formatToParts(items)
	if f.opts.Compat.Has(EmptyListItems) {
		parts = dropEmptyItems(parts)
	}
	return parts
}

// dropEmptyItems leaves out the items that are empty, joining the text
// either side of each, as ICU's FormattedList has no field for an empty
// span: ["a", "", "b"] is "a", ", , and ", "b".
func dropEmptyItems(parts []ListPart) []ListPart {
	out := parts[:0:0]
	for _, p := range parts {
		switch {
		case p.Kind == ListElement && p.Value == "":
		case p.Kind == ListLiteral && len(out) > 0 && out[len(out)-1].Kind == ListLiteral:
			out[len(out)-1].Value += p.Value
		default:
			out = append(out, p)
		}
	}
	return out
}

func (f *ListFormat) formatToParts(items []string) []ListPart {
	switch len(items) {
	case 0:
		return nil
	case 1:
		return []ListPart{{ListElement, items[0]}}
	case 2:
		return applyListPattern(f.two(items[1]),
			[]ListPart{{ListElement, items[0]}},
			[]ListPart{{ListElement, items[1]}})
	}

	if parts, ok := f.formatInOrder(items); ok {
		return parts
	}
	return f.formatFolding(items)
}

// formatFolding is FormatToParts as the patterns say it, each item folded in
// with the parts so far, which serves for any pattern.
func (f *ListFormat) formatFolding(items []string) []ListPart {
	parts := []ListPart{{ListElement, items[len(items)-1]}}
	for i := len(items) - 2; i >= 1; i-- {
		p := f.patterns.Middle
		if i == len(items)-2 {
			p = f.end(items[len(items)-1])
		}
		parts = applyListPattern(p, []ListPart{{ListElement, items[i]}}, parts)
	}
	return applyListPattern(f.patterns.Start,
		[]ListPart{{ListElement, items[0]}}, parts)
}

// formatInOrder is FormatToParts written from the front, for patterns that
// each put {0} before {1}, as every locale's do: what each pattern puts
// before, between and after them is written as the list is walked, and what
// they put after goes at the end, innermost first. Folding each item into
// the parts so far copied them for every item.
func (f *ListFormat) formatInOrder(items []string) ([]ListPart, bool) {
	start, ok1 := splitListPattern(f.patterns.Start)
	middle, ok2 := splitListPattern(f.patterns.Middle)
	end, ok3 := splitListPattern(f.end(items[len(items)-1]))
	if !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	out := make([]ListPart, 0, 3*len(items))
	literal := func(s string) {
		if s != "" {
			out = append(out, ListPart{ListLiteral, s})
		}
	}
	var after []string
	for i, item := range items[:len(items)-1] {
		p := middle
		switch i {
		case 0:
			p = start
		case len(items) - 2:
			p = end
		}
		literal(p[0])
		out = append(out, ListPart{ListElement, item})
		literal(p[1])
		after = append(after, p[2])
	}
	out = append(out, ListPart{ListElement, items[len(items)-1]})
	for i := len(after) - 1; i >= 0; i-- {
		literal(after[i])
	}
	return out, true
}

// splitListPattern is what a pattern puts before {0}, between {0} and {1},
// and after {1}; false for one that is not so simple.
func splitListPattern(pattern string) ([3]string, bool) {
	zero := strings.Index(pattern, "{0}")
	one := strings.Index(pattern, "{1}")
	if zero < 0 || one < zero+3 || strings.Contains(pattern[one+3:], "{") ||
		strings.Contains(pattern[:zero], "{") || strings.Contains(pattern[zero+3:one], "{") {
		return [3]string{}, false
	}
	return [3]string{pattern[:zero], pattern[zero+3 : one], pattern[one+3:]}, true
}

// applyListPattern fills a pattern's {0} and {1}, keeping what came from the
// items apart from what came from the pattern.
func applyListPattern(pattern string, first, second []ListPart) []ListPart {
	var out []ListPart
	literal := func(s string) {
		if s != "" {
			out = append(out, ListPart{ListLiteral, s})
		}
	}
	rest := pattern
	for {
		at := strings.IndexByte(rest, '{')
		if at < 0 || at+2 >= len(rest) || rest[at+2] != '}' {
			break
		}
		literal(rest[:at])
		switch rest[at+1] {
		case '0':
			out = append(out, first...)
		case '1':
			out = append(out, second...)
		default:
			literal(rest[at : at+3])
		}
		rest = rest[at+3:]
	}
	literal(rest)
	return out
}

// ResolvedListFormat is what a ListFormat settled on.
type ResolvedListFormat struct {
	Locale string
	Type   ListType
	Style  ListStyle
}

// ResolvedOptions returns what the formatter settled on.
func (f *ListFormat) ResolvedOptions() ResolvedListFormat {
	return ResolvedListFormat{
		Locale: f.locale.String(),
		Type:   f.opts.Type,
		Style:  f.opts.Style,
	}
}
