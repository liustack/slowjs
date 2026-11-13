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

// TestNumberDecimalMatchesNode holds FormatDecimal to what Node writes for
// numbers given as strings, which ECMA-402 reads exactly rather than as
// floats. The expectations are written by testdata/number_decimal_node.js.
func TestNumberDecimalMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/number_decimal_node.txt.gz")
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
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var tag, input, want string
		var opts map[string]any
		var wantParts [][2]string
		for i, dst := range []any{&tag, &opts, &input, &want, &wantParts} {
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
		if got := nf.FormatDecimal(intl.ParseDecimal(input)); got == want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s %q:\n   got  %+q\n   want %+q", key, input, got, want))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		if len(differences) > 25 && os.Getenv("ALLDIFF") == "" {
			differences = append(differences[:25], "...")
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(differences, "\n"))
	}
}
