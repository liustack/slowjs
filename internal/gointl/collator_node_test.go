//go:build gointl_all

package intl

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The collation gaps: what go-intl does not do, each with the reason, so that
// the test says exactly what it does not hold go-intl to. There are none; the
// last, Korean searchjl, closed when its jamo came from ICU's compiled data.
var collatorGaps = map[string]string{}

// TestCollatorMatchesNode holds every collation locale, under every option and
// collation type, to the order Node puts a word list in. The expectations are
// written by testdata/collator_node.js.
func TestCollatorMatchesNode(t *testing.T) {
	f, err := os.Open(filepath.FromSlash("testdata/collator_node.txt.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(make([]byte, 1<<20), 1<<20)

	var words []string
	orders := map[string]string{}
	var ran, matched, skipped int
	var differences []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		fields := strings.Split(line, "\t")
		switch fields[0] {
		case "words":
			if err := json.Unmarshal([]byte(fields[1]), &words); err != nil {
				t.Fatal(err)
			}
			continue
		case "order":
			orders[fields[1]] = fields[2]
			continue
		}
		tag, optsJSON, resolvedJSON, want := fields[1], fields[2], fields[3], orders[fields[4]]
		name := tag + " " + optsJSON
		if optsJSON == "{}" {
			name = tag
		}
		if _, gap := collatorGaps[name]; gap {
			skipped++
			continue
		}
		ran++

		opts, err := nodeCollatorOptions(optsJSON)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		loc, err := ParseLocale(tag)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c, err := NewCollator(loc, opts)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if diff := compareResolved(c.ResolvedOptions(), resolvedJSON); diff != "" {
			differences = append(differences, fmt.Sprintf("%s: resolved %s", name, diff))
		}

		// Each word's elements once, then the sort compares those.
		keys := make([][]uint64, len(words))
		for i, w := range words {
			keys[i] = c.sortElements(w, nil)
		}
		idx := make([]int, len(words))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(i, j int) bool {
			return c.compareElements(keys[idx[i]], keys[idx[j]]) < 0
		})
		var got strings.Builder
		got.WriteString(strconv.Itoa(idx[0]))
		for i := 1; i < len(idx); i++ {
			if c.compareElements(keys[idx[i-1]], keys[idx[i]]) == 0 {
				got.WriteByte('=')
			} else {
				got.WriteByte('<')
			}
			got.WriteString(strconv.Itoa(idx[i]))
		}
		if got.String() == want {
			matched++
			continue
		}
		differences = append(differences, name+": "+firstDifference(words, got.String(), want))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("no cases ran")
	}
	t.Logf("%d of %d orders match Node exactly; %d left out as known gaps", matched, ran, skipped)
	if len(differences) > 0 {
		shown := differences
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}

func nodeCollatorOptions(optsJSON string) (CollatorOptions, error) {
	opts := CollatorOptions{Compat: NodeICU}
	var raw map[string]any
	if err := json.Unmarshal([]byte(optsJSON), &raw); err != nil {
		return opts, err
	}
	for key, value := range raw {
		switch key {
		case "sensitivity":
			opts.Sensitivity = map[any]Sensitivity{"base": SensitivityBase, "accent": SensitivityAccent,
				"case": SensitivityCase, "variant": SensitivityVariant}[value]
		case "numeric":
			opts.Numeric = Bool(value == true)
		case "caseFirst":
			opts.CaseFirst = map[any]CaseFirst{"upper": CaseFirstUpper, "lower": CaseFirstLower,
				"false": CaseFirstFalse}[value]
		case "ignorePunctuation":
			opts.IgnorePunctuation = Bool(value == true)
		case "usage":
			if value == "search" {
				opts.Usage = UsageSearch
			}
		case "collation":
			opts.Collation, _ = value.(string)
		default:
			return opts, fmt.Errorf("unknown option %q", key)
		}
	}
	return opts, nil
}

func compareResolved(got ResolvedCollator, wantJSON string) string {
	var want map[string]any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		return err.Error()
	}
	have := map[string]any{
		"locale":            got.Locale,
		"usage":             got.Usage.String(),
		"sensitivity":       got.Sensitivity.String(),
		"ignorePunctuation": got.IgnorePunctuation,
		"collation":         got.Collation,
		"numeric":           got.Numeric,
		"caseFirst":         got.CaseFirst.String(),
	}
	var diffs []string
	for key, value := range have {
		if fmt.Sprint(want[key]) != fmt.Sprint(value) {
			diffs = append(diffs, fmt.Sprintf("%s is %v, want %v", key, value, want[key]))
		}
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "; ")
}

// firstDifference shows where two orders part, with a little on either side.
func firstDifference(words []string, got, want string) string {
	g, w := orderTokens(got), orderTokens(want)
	for i := range g {
		if i >= len(w) || g[i] != w[i] {
			lo := max(0, i-3)
			return fmt.Sprintf("\n   got  %s\n   want %s",
				showOrder(words, g[lo:min(len(g), i+4)]), showOrder(words, w[lo:min(len(w), i+4)]))
		}
	}
	return "the orders differ in length"
}

func orderTokens(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '<' || s[i] == '=' {
			out = append(out, s[start:i], s[i:i+1])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func showOrder(words, tokens []string) string {
	var b strings.Builder
	for _, tok := range tokens {
		if tok == "<" || tok == "=" {
			b.WriteString(" " + tok + " ")
			continue
		}
		n, _ := strconv.Atoi(tok)
		fmt.Fprintf(&b, "%+q", words[n])
	}
	return b.String()
}
