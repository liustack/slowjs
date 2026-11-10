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
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestDateTimeRangeMatchesNode holds formatRange and formatRangeToParts to
// what Node writes, in every locale it supports, for pairs of moments that
// differ in each field in turn. The expectations are written by
// testdata/datetime_range_node.js.
func TestDateTimeRangeMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/datetime_range_node.txt.gz")
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

	var ran, matched int
	var differences []string
	var lastKey string
	var last *intl.DateTimeFormat
	var lastErr error
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
		name := fmt.Sprintf("%s %s %v %v", tag, fields[1], int64(from), int64(to))
		if key := tag + string(fields[1]); key != lastKey {
			o, err := dateTimeOptions(opts)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			last, lastErr = intl.NewDateTimeFormat(loc, o)
			lastKey = key
		}
		if lastErr != nil {
			differences = append(differences, name+": "+lastErr.Error())
			continue
		}
		a, b := time.UnixMilli(int64(from)), time.UnixMilli(int64(to))
		got := last.FormatRange(a, b)
		var gotParts [][3]string
		for _, p := range last.FormatRangeToParts(a, b) {
			gotParts = append(gotParts, [3]string{string(p.Kind), p.Value, p.Source.String()})
		}
		if got == want && fmt.Sprint(gotParts) == fmt.Sprint(wantParts) {
			matched++
			continue
		}
		if got != want {
			differences = append(differences, fmt.Sprintf("%s\n   got  %+q\n   want %+q", name, got, want))
		} else {
			differences = append(differences, fmt.Sprintf("%s parts\n   got  %q\n   want %q", name, gotParts, wantParts))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}
