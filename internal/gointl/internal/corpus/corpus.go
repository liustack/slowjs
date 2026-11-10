//go:build gointl_all

// Package corpus reads the golden file that go-intl is held against.
//
// The file is what a full ICU answered for a corpus of formatting calls: each
// line is a JavaScript expression and the string it produced, separated by a
// tab. go-quickjs runs those expressions in its own engine. go-intl has no
// engine to run them in, so it takes them apart instead, into a service, a
// locale, an option bag and some arguments that a Go call can be made from.
//
// That is possible because the file is not really JavaScript. It was written
// by a generator that spells every value with JSON.stringify, so once the
// expression's shape is peeled away every literal left in it -- the strings,
// the numbers, the option bags, the string arrays -- is JSON, and the standard
// decoder reads them. Fifteen shapes account for the whole file.
package corpus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Kind is the shape of the expression a case is written as, which decides what
// its arguments mean.
type Kind int

const (
	// Format is a formatter called directly: the service is constructed with a
	// locale and options, and one method is called on it. NumberFormat,
	// DateTimeFormat, RelativeTimeFormat, ListFormat, PluralRules and
	// DisplayNames all take this shape, distinguished by Method.
	Format Kind = iota
	// Compare is Collator.compare called on one pair of strings.
	Compare
	// Sort is a list of strings sorted through a collator and joined, which is
	// how an ordering is written down as a string.
	Sort
	// Segment is a string broken by a segmenter, with one field of each
	// segment collected and joined.
	Segment
	// ToLocale is one of the legacy methods -- Number.prototype.toLocaleString
	// and the three on Date -- which format without a named formatter.
	ToLocale
)

var kindNames = [...]string{"Format", "Compare", "Sort", "Segment", "ToLocale"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// A Case is one line of the golden file.
type Case struct {
	// Line is where it came from, and Source is the expression as written, so
	// that a difference can be reported against the file rather than against
	// the pieces it was taken apart into.
	Line   int
	Source string
	// Want is what ICU answered.
	Want string

	Kind Kind
	// Service is the Intl service, "NumberFormat" and so on. It is empty for
	// ToLocale, which names no service.
	Service string
	// Method is the method called: "format", "select", "of", "compare", or one
	// of the toLocale names.
	Method string
	// Locale is the tag asked for, and Options the bag beside it, which is nil
	// when the call passed none.
	Locale  string
	Options map[string]any

	// Args are the method's arguments, each a float64, a string, or a
	// []string, in the order they were written.
	Args []any

	// Field is which part of a segment the Segment kind collects: "segment",
	// "index", or "isWordLike". Join is the separator that kind and Sort use
	// to write their result as one string.
	Field string
	Join  string
	// IfWordLike and IfNot are the branches of a conditional on isWordLike,
	// when the map body is one: s => s.isWordLike ? "w" : ".".
	IfWordLike, IfNot string
}

// Number returns the case's first numeric argument.
func (c *Case) Number() (float64, bool) {
	for _, a := range c.Args {
		if n, ok := a.(float64); ok {
			return n, true
		}
	}
	return 0, false
}

// Strings returns the case's first []string argument, which is the list a
// ListFormat joins or the items a collator sorts.
func (c *Case) Strings() ([]string, bool) {
	for _, a := range c.Args {
		if s, ok := a.([]string); ok {
			return s, true
		}
	}
	return nil, false
}

// A File is the parsed golden file together with what produced it.
type File struct {
	// ICU is the version named in the header comment, which is the only thing
	// "matches ICU exactly" can mean.
	ICU   string
	Cases []*Case
}

// Load reads the golden file at path.
func Load(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// Read parses the golden file from r.
func Read(r io.Reader) (*File, error) {
	out := &File{}
	scan := bufio.NewScanner(r)
	// A collation case lists every string it sorts, so a line is long.
	scan.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 0; scan.Scan(); {
		line++
		text := scan.Text()
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "#") {
			if v := strings.TrimSpace(strings.TrimPrefix(text, "#")); out.ICU == "" {
				out.ICU = strings.TrimPrefix(v, "ICU ")
			}
			continue
		}
		source, quoted, ok := strings.Cut(text, "\t")
		if !ok {
			return nil, fmt.Errorf("line %d: no tab between the call and its answer", line)
		}
		want, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: unquoting the answer: %w", line, source, err)
		}
		c, err := parse(source)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", line, source, err)
		}
		c.Line, c.Source, c.Want = line, source, want
		out.Cases = append(out.Cases, c)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if len(out.Cases) == 0 {
		return nil, fmt.Errorf("the golden file has no cases")
	}
	return out, nil
}

// parse takes one expression apart. The shape is decided by how it begins,
// which separates the five kinds without any backtracking.
func parse(src string) (*Case, error) {
	switch {
	case strings.HasPrefix(src, "new Intl."):
		return parseService(src)
	case strings.HasPrefix(src, "[...new Intl.Segmenter("):
		return parseSegment(src)
	case strings.HasPrefix(src, "["):
		return parseSort(src)
	case strings.HasPrefix(src, "("), strings.HasPrefix(src, "new Date("):
		return parseToLocale(src)
	}
	return nil, fmt.Errorf("unrecognized call")
}

// parseService reads new Intl.Service(locale, options).method(args...).
func parseService(src string) (*Case, error) {
	c := &Case{Kind: Format}
	rest, err := c.readConstructor(strings.TrimPrefix(src, "new Intl."))
	if err != nil {
		return nil, err
	}
	method, args, rest, err := readCall(rest)
	if err != nil {
		return nil, err
	}
	if rest != "" {
		return nil, fmt.Errorf("trailing %q after .%s()", rest, method)
	}
	c.Method = method
	if c.Args, err = decodeArgs(args); err != nil {
		return nil, err
	}
	if method == "compare" {
		c.Kind = Compare
	}
	return c, nil
}

// parseSort reads [items].sort(new Intl.Collator(locale, options).compare).join(sep).
func parseSort(src string) (*Case, error) {
	items, rest, err := readBracket(src)
	if err != nil {
		return nil, err
	}
	// readBracket strips the brackets and the decoder wants an array, so they
	// go back on rather than the list being taken apart a second way.
	list, err := decodeStrings("[" + items + "]")
	if err != nil {
		return nil, err
	}
	inner, rest, err := readParens(strings.TrimPrefix(rest, ".sort"))
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(inner, "new Intl.") {
		return nil, fmt.Errorf("sort is not given an Intl comparator")
	}
	c := &Case{Kind: Sort, Args: []any{list}}
	after, err := c.readConstructor(strings.TrimPrefix(inner, "new Intl."))
	if err != nil {
		return nil, err
	}
	if after != ".compare" {
		return nil, fmt.Errorf("sort is given %q rather than .compare", after)
	}
	c.Method = "compare"
	if c.Join, err = readJoin(rest); err != nil {
		return nil, err
	}
	return c, nil
}

// parseSegment reads
// [...new Intl.Segmenter(locale, options).segment(s)].map(s => s.field).join(sep),
// where the field may instead be a conditional on isWordLike.
func parseSegment(src string) (*Case, error) {
	inner, rest, err := readBracket(src)
	if err != nil {
		return nil, err
	}
	c := &Case{Kind: Segment}
	after, err := c.readConstructor(strings.TrimPrefix(inner, "...new Intl."))
	if err != nil {
		return nil, err
	}
	method, args, after, err := readCall(after)
	if err != nil {
		return nil, err
	}
	if method != "segment" || after != "" {
		return nil, fmt.Errorf("the spread is not a plain .segment() call")
	}
	c.Method = method
	if c.Args, err = decodeArgs(args); err != nil {
		return nil, err
	}

	body, rest, err := readParens(strings.TrimPrefix(rest, ".map"))
	if err != nil {
		return nil, err
	}
	// The body is either s => s.field or s => s.isWordLike ? "a" : "b".
	_, field, ok := strings.Cut(body, "s.")
	if !ok {
		return nil, fmt.Errorf("the map body reads no field of the segment")
	}
	if at := strings.IndexAny(field, " ?"); at >= 0 {
		if cond := strings.TrimSpace(field[at:]); strings.HasPrefix(cond, "?") {
			yes, no, ok := strings.Cut(strings.TrimPrefix(cond, "?"), ":")
			if !ok {
				return nil, fmt.Errorf("a conditional without two branches")
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(yes)), &c.IfWordLike); err != nil {
				return nil, fmt.Errorf("the conditional's first branch: %w", err)
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(no)), &c.IfNot); err != nil {
				return nil, fmt.Errorf("the conditional's second branch: %w", err)
			}
		}
		field = field[:at]
	}
	c.Field = field
	if c.Join, err = readJoin(rest); err != nil {
		return nil, err
	}
	return c, nil
}

// parseToLocale reads (n).toLocaleString(...) and new Date(n).toLocaleX(...).
func parseToLocale(src string) (*Case, error) {
	receiver, rest, err := readParens(strings.TrimPrefix(src, "new Date"))
	if err != nil {
		return nil, err
	}
	c := &Case{Kind: ToLocale}
	if c.Args, err = decodeArgs([]string{receiver}); err != nil {
		return nil, err
	}
	method, args, rest, err := readCall(rest)
	if err != nil {
		return nil, err
	}
	if rest != "" {
		return nil, fmt.Errorf("trailing %q after .%s()", rest, method)
	}
	c.Method = method
	if len(args) > 0 {
		if c.Locale, err = decodeString(args[0]); err != nil {
			return nil, err
		}
	}
	if len(args) > 1 {
		if c.Options, err = decodeOptions(args[1]); err != nil {
			return nil, err
		}
	}
	if len(args) > 2 {
		return nil, fmt.Errorf(".%s takes at most a locale and options", method)
	}
	return c, nil
}

// readConstructor reads Service(locale, options) from the front of src, having
// had "new Intl." removed, and returns what follows it.
func (c *Case) readConstructor(src string) (string, error) {
	open := strings.IndexByte(src, '(')
	if open < 0 {
		return "", fmt.Errorf("the constructor names no service")
	}
	c.Service = src[:open]
	args, rest, err := readParens(src[open:])
	if err != nil {
		return "", err
	}
	fields, err := splitArgs(args)
	if err != nil {
		return "", err
	}
	if len(fields) == 0 {
		return "", fmt.Errorf("%s is constructed without a locale", c.Service)
	}
	if c.Locale, err = decodeString(fields[0]); err != nil {
		return "", err
	}
	if len(fields) > 1 {
		if c.Options, err = decodeOptions(fields[1]); err != nil {
			return "", err
		}
	}
	if len(fields) > 2 {
		return "", fmt.Errorf("%s is constructed with %d arguments", c.Service, len(fields))
	}
	return rest, nil
}

// readCall reads .name(args...) from the front of src.
func readCall(src string) (name string, args []string, rest string, err error) {
	if !strings.HasPrefix(src, ".") {
		return "", nil, "", fmt.Errorf("expected a method call, found %q", src)
	}
	open := strings.IndexByte(src, '(')
	if open < 0 {
		return "", nil, "", fmt.Errorf("the method is never called")
	}
	name = src[1:open]
	inner, rest, err := readParens(src[open:])
	if err != nil {
		return "", nil, "", err
	}
	if args, err = splitArgs(inner); err != nil {
		return "", nil, "", err
	}
	return name, args, rest, nil
}

// readJoin reads a trailing .join(sep).
func readJoin(src string) (string, error) {
	name, args, rest, err := readCall(src)
	if err != nil {
		return "", err
	}
	if name != "join" || rest != "" || len(args) != 1 {
		return "", fmt.Errorf("expected one trailing .join(), found %q", src)
	}
	return decodeString(args[0])
}

// readParens returns what one bracketed group holds and what follows it. The
// scan counts nesting and skips over strings, so a bracket inside a string --
// the separator "(" would be one -- does not end the group early.
func readParens(src string) (inner, rest string, err error) {
	return readGroup(src, '(', ')')
}

func readBracket(src string) (inner, rest string, err error) {
	return readGroup(src, '[', ']')
}

func readGroup(src string, open, close byte) (inner, rest string, err error) {
	if len(src) == 0 || src[0] != open {
		return "", "", fmt.Errorf("expected %q, found %q", string(open), src)
	}
	depth := 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '"':
			end, err := skipString(src, i)
			if err != nil {
				return "", "", err
			}
			i = end
		case open:
			depth++
		case close:
			if depth--; depth == 0 {
				return src[1:i], src[i+1:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unbalanced %q", string(open))
}

// skipString returns the index of the quote that closes the one at start.
func skipString(src string, start int) (int, error) {
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '"':
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated string")
}

// splitArgs divides an argument list on the commas that separate arguments,
// leaving alone those inside a string, an object, or an array.
func splitArgs(src string) ([]string, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '"':
			end, err := skipString(src, i)
			if err != nil {
				return nil, err
			}
			i = end
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(src[start:i]))
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced brackets in %q", src)
	}
	return append(out, strings.TrimSpace(src[start:])), nil
}

// decodeArgs turns written arguments into values. Everything the generator
// writes is JSON, so the difference between a number, a string and a list is
// the first character.
func decodeArgs(args []string) ([]any, error) {
	var out []any
	for _, a := range args {
		switch {
		case a == "":
			continue
		case a[0] == '"':
			s, err := decodeString(a)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		case a[0] == '[':
			s, err := decodeStrings(a)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		default:
			n, err := strconv.ParseFloat(a, 64)
			if err != nil {
				return nil, fmt.Errorf("argument %q is not a number: %w", a, err)
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func decodeString(src string) (string, error) {
	var s string
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		return "", fmt.Errorf("decoding the string %q: %w", src, err)
	}
	return s, nil
}

func decodeStrings(src string) ([]string, error) {
	var s []string
	if err := json.Unmarshal([]byte(src), &s); err != nil {
		return nil, fmt.Errorf("decoding the list %q: %w", src, err)
	}
	return s, nil
}

func decodeOptions(src string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(src), &m); err != nil {
		return nil, fmt.Errorf("decoding the options %q: %w", src, err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}
