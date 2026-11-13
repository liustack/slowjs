//go:build gointl_all

package intl

import (
	"fmt"
	"math"
	"math/rand"
	"runtime/debug"
	"testing"
	"time"
)

// Corrupt break rules, sets and dictionaries are refused when the
// segmenter is built, or break text somehow, but never panic or loop
// (ISSUES.md DA-4).
func TestCorruptSegmenterData(t *testing.T) {
	texts := []string{
		"Hello, world. How are you? 123.45 e-mail@example.com",
		"สวัสดีครับ ภาษาไทย",
		"ພາສາລາວ ភាសាខ្មែរ",
		"မြန်မာဘာသာ",
		"日本語の文章です。中文句子。",
		"Καλημέρα; Τι κάνεις;",
		"\U0001F468‍\U0001F469‍\U0001F467 \U0001F1EF\U0001F1F5 ẹ́ \xed\xa0\x80x",
	}
	type target struct {
		file, tag string
		g         Granularity
	}
	targets := []target{
		{"char", "en", GranularityGrapheme}, {"word", "en", GranularityWord},
		{"word_POSIX", "en-US-POSIX", GranularityWord}, {"sent", "en", GranularitySentence},
		{"sent_el", "el", GranularitySentence}, {"sets", "th", GranularityWord},
		{"boundaries", "en", GranularityWord}, {"thaidict", "th", GranularityWord},
		{"laodict", "lo", GranularityWord}, {"khmerdict", "km", GranularityWord},
		{"burmesedict", "my", GranularityWord}, {"cjdict", "ja", GranularityWord},
	}
	built, refused := 0, 0
	for _, tg := range targets {
		loc := mustParse(t, tg.tag)
		m := Marker("brkitr/" + tg.file)
		orig, err := Embedded.Open(m, DataLocale{})
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		rng := rand.New(rand.NewSource(int64(len(orig))))
		for iter := 0; iter < 150; iter++ {
			b := append([]byte(nil), orig...)
			for flips := 1 + rng.Intn(8); flips > 0; flips-- {
				at := rng.Intn(len(b))
				if iter%3 == 0 {
					// A third of the changes are to the header, where the
					// layout is.
					at = rng.Intn(min(len(b), 96))
				}
				b[at] ^= byte(1 + rng.Intn(255))
			}
			if iter%10 == 9 {
				b = b[:rng.Intn(len(b))]
			}
			src := corruptSource{Embedded, m, DataLocale{}, b}
			name := fmt.Sprintf("%s, change %d", m, iter)
			done := make(chan string, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Sprintf("panicked: %v\n%s", r, debug.Stack())
					}
				}()
				s, err := NewSegmenterFrom(src, loc, SegmenterOptions{Granularity: tg.g})
				if err != nil {
					done <- "refused"
					return
				}
				for _, text := range texts {
					segs := s.SegmentString(text)
					segs.All()
					for n := 0; n < len(text); n += 3 {
						segs.Containing(n)
					}
				}
				done <- ""
			}()
			select {
			case r := <-done:
				switch r {
				case "":
					built++
				case "refused":
					refused++
				default:
					t.Fatalf("%s: %s", name, r)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s: still breaking after ten seconds", name)
			}
		}
	}
	if built == 0 {
		t.Errorf("every corrupt file was refused (%d): the test reached no text", refused)
	}
	t.Logf("%d corrupt files built and broken with, %d refused", built, refused)
}

// A dictionary whose trie starts after it ends is refused: it had been
// sliced, and panicked (DA-4).
func TestDictionaryOffsetPastTotal(t *testing.T) {
	b := make([]byte, 64)
	put := func(i, v int) { b[4*i], b[4*i+1], b[4*i+2], b[4*i+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24) }
	put(0, 48) // the trie's offset
	put(3, 40) // the end of the data, before it
	put(4, 1)  // a trie of 16-bit units
	if _, err := decodeBreakDictionary(b); err == nil {
		t.Error("a dictionary ending before its trie starts was read")
	}
	put(3, 64)
	if _, err := decodeBreakDictionary(b); err != nil {
		t.Errorf("a well-formed header: %v", err)
	}
}

// A status group whose count runs past the table is refused rather than
// read. The count is compared with what is left, never added to the index:
// on 32 bits a count past 2^31 is negative and idx+n wraps back into the
// table, which the random changes above would hit only by luck.
func TestRuleStatusCountPastTable(t *testing.T) {
	r := &breakRules{statuses: []int32{2, 5, 9, math.MaxInt32, 3}}
	// A well-formed group: two values after the count, the last of them.
	if got := r.ruleStatus(0); got != 9 {
		t.Errorf("a group of two values is %d, want 9", got)
	}
	for _, idx := range []int{3, 4, -1, len(r.statuses)} {
		if got := r.ruleStatus(idx); got != 0 {
			t.Errorf("status %d of %d is %d, want 0", idx, len(r.statuses), got)
		}
	}
}
