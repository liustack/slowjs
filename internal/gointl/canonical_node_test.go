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

// TestCanonicalizeMatchesNode replays testdata/canonical_node.js: every alias
// ICU knows, in a few surroundings, every Unicode extension type, and
// test262's canonicalization tags, against Intl.getCanonicalLocales.
func TestCanonicalizeMatchesNode(t *testing.T) {
	c, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{Compat: intl.NodeICU})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("testdata/canonical_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
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
		var c2 [2]*string
		if err := json.Unmarshal([]byte(line), &c2); err != nil || c2[0] == nil {
			t.Fatalf("a line that is not a tag and its answer: %s", line)
		}
		tag := *c2[0]
		ran++
		got, err := c.Canonicalize(tag)
		switch {
		case c2[1] == nil && err != nil:
			matched++
		case c2[1] == nil:
			diffs = append(diffs, fmt.Sprintf("%q: got %q, Node throws", tag, got.String()))
		case err != nil:
			diffs = append(diffs, fmt.Sprintf("%q: %v, want %q", tag, err, *c2[1]))
		case got.String() != *c2[1]:
			diffs = append(diffs, fmt.Sprintf("%q: got %q, want %q", tag, got.String(), *c2[1]))
		default:
			matched++
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
