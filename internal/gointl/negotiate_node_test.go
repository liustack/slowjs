//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// TestAvailableLocalesMatchNode replays testdata/available_node.js: for every
// name ICU has a bundle for, cut short and without its script, whether each
// service resolves it as itself.
func TestAvailableLocalesMatchNode(t *testing.T) {
	f, err := os.Open("testdata/available_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	matchers := map[string]*intl.LocaleMatcher{}
	s := bufio.NewScanner(z)
	var ran, matched int
	var diffs []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var service, tag string
		var want bool
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 3 {
			t.Fatalf("a line that is not three fields: %s", line)
		}
		json.Unmarshal(c[0], &service)
		json.Unmarshal(c[1], &tag)
		json.Unmarshal(c[2], &want)
		m, ok := matchers[service]
		if !ok {
			if m, err = intl.NewLocaleMatcher(intl.Embedded, intl.Service(service)); err != nil {
				t.Fatal(err)
			}
			matchers[service] = m
		}
		ran++
		if got := m.Available(tag); got == want {
			matched++
		} else {
			diffs = append(diffs, fmt.Sprintf("%s %s: available %v, Node %v", service, tag, got, want))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node", matched, ran)
	if len(diffs) > 0 {
		if len(diffs) > 60 {
			diffs = diffs[:60]
		}
		t.Errorf("%d differ, the first:\n\t%s", ran-matched, strings.Join(diffs, "\n\t"))
	}
}
