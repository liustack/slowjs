//go:build gointl_all

package intl

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestTimeZonesMatchNode replays testdata/timezone_node.js: the names
// Intl.DateTimeFormat accepts and resolves for every name ICU knows, in
// three spellings, and every zone's offsets from 1800 to 2100, transition by
// transition, as Temporal reports them.
func TestTimeZonesMatchNode(t *testing.T) {
	f, err := os.Open("testdata/timezone_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(gz)
	s.Buffer(nil, 1<<20)
	start := gregoDay(1800, 0, 1) * msPerDay
	end := gregoDay(2100, 0, 1) * msPerDay
	ids, zones, bad := 0, 0, 0
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.Contains(line, "ICU 78.3, tz 2026c") {
				t.Fatalf("the expectations come from %q, not ICU 78.3 with tz 2026c", line)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) < 3 {
			t.Fatalf("a line of the wrong shape: %s", line)
		}
		var kind, name string
		json.Unmarshal(c[0], &kind)
		json.Unmarshal(c[1], &name)
		fail := func(format string, args ...any) {
			bad++
			if bad <= 40 {
				t.Errorf("%s: "+format, append([]any{name}, args...)...)
			}
		}
		switch kind {
		case "id":
			if name == "" {
				// An empty option is one not given, in Go, and so UTC.
				continue
			}
			ids++
			var want *string
			json.Unmarshal(c[2], &want)
			df, err := NewDateTimeFormat(Locale{}, DateTimeFormatOptions{
				// Node's names, ICU's canonical ones.
				TimeZone: name, Compat: ZoneIdentifiers,
			})
			switch {
			case want == nil && err == nil:
				fail("resolved as %q, want an error", df.ResolvedOptions().TimeZone)
			case want != nil && err != nil:
				fail("%v, want %q", err, *want)
			case want != nil && df.ResolvedOptions().TimeZone != *want:
				fail("resolved as %q, want %q", df.ResolvedOptions().TimeZone, *want)
			}
		case "zone":
			zones++
			var offset *int
			var trans []int64
			json.Unmarshal(c[2], &offset)
			json.Unmarshal(c[3], &trans)
			if offset == nil {
				// A zone Temporal does not take, which Intl.DateTimeFormat
				// may: Java's three-letter names, SystemV's.
				continue
			}
			z, err := loadTimeZone(Embedded, name)
			if err != nil {
				fail("%v", err)
				continue
			}
			if got := z.offsetAt(start).total(); got != *offset {
				fail("offset %d in 1800, want %d", got, *offset)
				continue
			}
			// Temporal reports a transition only where the offset from UTC
			// changes.
			var got []int64
			prev := *offset
			for at := start; ; {
				tr, ok := z.nextTransition(at, false)
				if !ok || tr.at >= end {
					break
				}
				at = tr.at
				if tr.to.total() != prev {
					got = append(got, tr.at, int64(tr.to.total()))
					prev = tr.to.total()
				}
				if tr.at < start {
					continue
				}
				if o := z.offsetAt(tr.at).total(); o != tr.to.total() {
					fail("offset %d at the transition at %d, which is to %d", o, tr.at, tr.to.total())
				}
				if o := z.offsetAt(tr.at - 1).total(); o != tr.from.total() {
					fail("offset %d before the transition at %d, which is from %d", o, tr.at, tr.from.total())
				}
			}
			if len(got) != len(trans) {
				fail("%d transitions, want %d", len(got)/2, len(trans)/2)
			}
			for i := 0; i < len(got) && i < len(trans); i += 2 {
				if got[i] != trans[i] || got[i+1] != trans[i+1] {
					fail("transition %d at %d to %d, want at %d to %d", i/2, got[i], got[i+1], trans[i], trans[i+1])
					break
				}
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if ids < 1800 || zones < 400 {
		t.Fatalf("%d names and %d zones: the recording is short", ids, zones)
	}
	t.Logf("%d names, %d zones, %d differ", ids, zones, bad)
}
