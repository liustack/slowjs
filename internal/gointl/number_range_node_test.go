//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestNumberRangeMatchesNode holds FormatRange and FormatRangeToParts to what
// Node writes. The expectations are written by testdata/number_range_node.js.
func TestNumberRangeMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/number_range_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	formats := map[string]*intl.NumberFormat{}
	var ran, matched int
	var differences []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 6 {
			t.Fatalf("a line that is not six fields: %s", line)
		}
		var tag, want string
		var opts map[string]any
		var from, to float64
		var wantParts [][3]string
		for i, dst := range []any{&tag, &opts, &from, &to, &want, &wantParts} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++
		key := tag + string(fields[1])
		nf, ok := formats[key]
		if !ok {
			o, err := numberOptions(opts)
			if err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			o.Compat = intl.NodeICU
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			if nf, err = intl.NewNumberFormat(loc, o); err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			formats[key] = nf
		}
		name := fmt.Sprintf("%s %v–%v", key, from, to)
		got, err := nf.FormatRange(from, to)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		parts, _ := nf.FormatRangeToParts(from, to)
		var gotParts [][3]string
		for _, p := range parts {
			gotParts = append(gotParts, [3]string{string(p.Kind), p.Value, p.Source.String()})
		}
		switch {
		case got != want:
			differences = append(differences, fmt.Sprintf("%s\n   got  %+q\n   want %+q", name, got, want))
		case fmt.Sprint(gotParts) != fmt.Sprint(wantParts):
			differences = append(differences, fmt.Sprintf("%s parts\n   got  %q\n   want %q", name, gotParts, wantParts))
		default:
			matched++
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		if len(differences) > 30 && os.Getenv("ALLDIFF") == "" {
			differences = append(differences[:30], "...")
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(differences, "\n"))
	}
}
