package intl

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
)

// ICU's dictionary break engines (dictbe.cpp): Thai, Lao, Burmese and
// Khmer by looking a few dictionary words ahead, and Chinese and Japanese
// by the cheapest segmentation in a dictionary of word costs.

// A breakEngine is LanguageBreakEngine.
type breakEngine interface {
	handles(c rune) bool
	// findBreaks is LanguageBreakEngine::findBreaks: the breaks in the run
	// of the engine's characters from start, added to breaks, and the
	// iterator left after the run.
	findBreaks(t *utext, start, end int, breaks *[]int) int
}

// breakEngines are the engines ICU's factory makes, and the Script
// property it chooses among them by.
type breakEngines struct {
	engines []breakEngine
	// byScript is the engine for each script with a dictionary.
	byScript map[string]breakEngine
	scripts  scriptTable
}

// forChar is the factory's engine for a character: the one whose set has
// it, else the one for its script.
func (e *breakEngines) forChar(c rune) breakEngine {
	for _, x := range e.engines {
		if x.handles(c) {
			return x
		}
	}
	return e.byScript[e.script(c)]
}

// script is uscript_getScript, by the short names ICU keeps dictionaries
// under where it has them.
func (e *breakEngines) script(c rune) string { return e.scripts.script(c) }

// scriptTable is the Script property, read where it lies: every script's
// ranges, sorted, each the first and last code point and the script's
// number, which counts the names.
type scriptTable struct {
	ranges []byte // twelve bytes a range
	names  []string
}

func (t scriptTable) script(c rune) string {
	at := func(i, field int) rune { return rune(binary.LittleEndian.Uint32(t.ranges[12*i+4*field:])) }
	n := len(t.ranges) / 12
	i := sort.Search(n, func(i int) bool { return at(i, 1) >= c })
	if i == n || at(i, 0) > c {
		return "Unknown"
	}
	// A number past 2^31, as a corrupt table may hold, is a negative rune.
	id := int(at(i, 2))
	if id < 0 || id >= len(t.names) {
		return "Unknown"
	}
	if code, ok := scriptCodes[t.names[id]]; ok {
		return code
	}
	return t.names[id]
}

// scriptCodes are the ISO 15924 codes ICU names dictionaries by, for the
// scripts Scripts.txt names in full.
var scriptCodes = map[string]string{
	"Thai": "Thai", "Lao": "Laoo", "Myanmar": "Mymr", "Khmer": "Khmr",
	"Han": "Hani", "Hiragana": "Hira", "Katakana": "Kana", "Hangul": "Hang",
}

func loadBreakEngines(src Source, norm *Normalizer) (*breakEngines, error) {
	b, err := src.Open(Marker("brkitr/sets"), DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the break engines' sets: %w", err)
	}
	index, err := blob.ReadIndex(b, layout.BreakSets)
	if err != nil {
		return nil, fmt.Errorf("intl: the break engines' sets: %w", err)
	}
	sets := map[string]codeRanges{}
	for _, name := range []string{"thai", "thaimarks", "lao", "laomarks", "khmer", "khmermarks",
		"myanmar", "myanmarmarks", "cj"} {
		v, _ := index.Find("set " + name)
		if len(v)%8 != 0 {
			return nil, fmt.Errorf("intl: the break engines' set %s is %d bytes", name, len(v))
		}
		set := make(codeRanges, len(v)/8)
		for i := range set {
			set[i] = [2]rune{rune(binary.LittleEndian.Uint32(v[8*i:])), rune(binary.LittleEndian.Uint32(v[8*i+4:]))}
		}
		sets[name] = set
	}
	ranges, _ := index.Find("script ranges")
	names, _ := index.Find("script names")
	if len(ranges)%12 != 0 {
		return nil, fmt.Errorf("intl: the script table is %d bytes", len(ranges))
	}
	scripts := scriptTable{ranges: ranges, names: strings.Split(string(names), "\n")}
	dicts := map[string]string{}
	if b, err = src.Open(Marker("brkitr/boundaries"), DataLocale{}); err != nil {
		return nil, fmt.Errorf("intl: the break rules: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) == 3 && f[0] == "dictionary" {
			dicts[f[1]] = strings.TrimSuffix(f[2], ".dict")
		}
	}
	load := func(script string) (*breakDictionary, error) {
		name, ok := dicts[script]
		if !ok {
			return nil, nil
		}
		b, err := src.Open(Marker("brkitr/"+name), DataLocale{})
		if err != nil {
			return nil, fmt.Errorf("intl: the %s dictionary: %w", name, err)
		}
		return decodeBreakDictionary(b)
	}
	out := &breakEngines{byScript: map[string]breakEngine{}, scripts: scripts}
	space := func(s codeRanges) codeRanges { return s.with(0x20, 0x20) }
	add := func(script string, make func(d *breakDictionary) breakEngine) error {
		d, err := load(script)
		if err != nil || d == nil {
			return err
		}
		e := make(d)
		out.engines = append(out.engines, e)
		out.byScript[script] = e
		return nil
	}
	if err := add("Thai", func(d *breakDictionary) breakEngine {
		return &southeastAsianEngine{dict: d, set: sets["thai"], marks: space(sets["thaimarks"]),
			endWord:   sets["thai"].without(0x0e31, 0x0e31).without(0x0e40, 0x0e44),
			beginWord: codeRanges{{0x0e01, 0x0e2e}, {0x0e40, 0x0e44}},
			suffixes:  true, minSpanInCodePoints: true}
	}); err != nil {
		return nil, err
	}
	if err := add("Laoo", func(d *breakDictionary) breakEngine {
		return &southeastAsianEngine{dict: d, set: sets["lao"], marks: space(sets["laomarks"]),
			endWord:   sets["lao"].without(0x0ec0, 0x0ec4),
			beginWord: codeRanges{{0x0e81, 0x0eae}, {0x0ec0, 0x0ec4}, {0x0edc, 0x0edd}}}
	}); err != nil {
		return nil, err
	}
	if err := add("Mymr", func(d *breakDictionary) breakEngine {
		return &southeastAsianEngine{dict: d, set: sets["myanmar"], marks: space(sets["myanmarmarks"]),
			endWord: sets["myanmar"], beginWord: codeRanges{{0x1000, 0x102a}}}
	}); err != nil {
		return nil, err
	}
	if err := add("Khmr", func(d *breakDictionary) breakEngine {
		return &southeastAsianEngine{dict: d, set: sets["khmer"], marks: space(sets["khmermarks"]),
			endWord: sets["khmer"].without(0x17d2, 0x17d2), beginWord: codeRanges{{0x1780, 0x17b3}}}
	}); err != nil {
		return nil, err
	}
	cj, err := load("Hani")
	if err != nil {
		return nil, err
	}
	if cj != nil {
		e := newCJKEngine(cj, sets["cj"], norm)
		out.engines = append(out.engines, e)
		for _, s := range []string{"Hani", "Hira", "Kana"} {
			if _, ok := dicts[s]; ok {
				out.byScript[s] = e
			}
		}
	}
	return out, nil
}

// possibleWord is PossibleWord: the dictionary words at one position,
// longest first, and the one preferred.
type possibleWord struct {
	count, prefix, offset, mark, current int
	cuLengths, cpLengths                 [20]int
}

// candidates fills the list if needed, selects the longest, and returns
// how many there are.
func (p *possibleWord) candidates(t *utext, d *breakDictionary, end int) int {
	start := t.index()
	if start != p.offset {
		p.offset = start
		p.count, p.prefix = d.matches(t, end-start, len(p.cuLengths), p.cuLengths[:], p.cpLengths[:], nil)
		if p.count <= 0 {
			t.setIndex(start)
		}
	}
	if p.count > 0 {
		t.setIndex(start + p.cuLengths[p.count-1])
	}
	p.current = p.count - 1
	p.mark = p.current
	return p.count
}

func (p *possibleWord) acceptMarked(t *utext) int {
	t.setIndex(p.offset + p.cuLengths[p.mark])
	return p.cuLengths[p.mark]
}

func (p *possibleWord) backUp(t *utext) bool {
	if p.current > 0 {
		p.current--
		t.setIndex(p.offset + p.cuLengths[p.current])
		return true
	}
	return false
}

func (p *possibleWord) markCurrent()        { p.mark = p.current }
func (p *possibleWord) markedCPLength() int { return p.cpLengths[p.mark] }

// southeastAsianEngine is ThaiBreakEngine, LaoBreakEngine,
// BurmeseBreakEngine and KhmerBreakEngine, which are one algorithm with
// their own characters: look up to three dictionary words ahead, join a
// non-word to the short word before it, and never stop before a mark.
type southeastAsianEngine struct {
	dict                           *breakDictionary
	set, marks, endWord, beginWord codeRanges
	// suffixes is Thai's: PAIYANNOI and MAIYAMOK join the word before.
	suffixes bool
	// minSpanInCodePoints is Thai's: two words' worth of characters is
	// counted in code points, where the others count code units.
	minSpanInCodePoints bool
}

const (
	seaLookahead          = 3
	seaRootCombine        = 3
	seaPrefixCombine      = 3
	seaMinWordSpan        = 4
	thaiPaiyannoi    rune = 0x0e2f
	thaiMaiyamok     rune = 0x0e46
)

func (e *southeastAsianEngine) handles(c rune) bool { return e.set.contains(c) }

func (e *southeastAsianEngine) findBreaks(t *utext, start, end int, breaks *[]int) int {
	return dictionaryFindBreaks(t, start, end, breaks, e.set, e.divide)
}

// dictionaryFindBreaks is DictionaryBreakEngine::findBreaks: the run of
// the engine's characters from start, divided.
func dictionaryFindBreaks(t *utext, start, end int, breaks *[]int, set codeRanges,
	divide func(t *utext, start, end int, breaks *[]int) int) int {
	t.setIndex(start)
	begin := t.index()
	c := t.current32()
	current := t.index()
	for current < end && set.contains(c) {
		t.next32()
		c = t.current32()
		current = t.index()
	}
	n := divide(t, begin, current, breaks)
	t.setIndex(current)
	return n
}

func peek(breaks []int) int {
	if len(breaks) == 0 {
		return 0
	}
	return breaks[len(breaks)-1]
}

func (e *southeastAsianEngine) divide(t *utext, rangeStart, rangeEnd int, breaks *[]int) int {
	if e.minSpanInCodePoints {
		t.setIndex(rangeStart)
		t.moveIndex32(seaMinWordSpan)
		if t.index() >= rangeEnd {
			return 0
		}
	} else if rangeEnd-rangeStart < seaMinWordSpan {
		return 0
	}
	var words [seaLookahead]possibleWord
	for i := range words {
		words[i].offset = -1
	}
	wordsFound := 0
	t.setIndex(rangeStart)
	for {
		current := t.index()
		if current >= rangeEnd {
			break
		}
		cpWordLength, cuWordLength := 0, 0
		w := &words[wordsFound%seaLookahead]
		candidates := w.candidates(t, e.dict, rangeEnd)
		if candidates == 1 {
			cuWordLength = w.acceptMarked(t)
			cpWordLength = w.markedCPLength()
			wordsFound++
		} else if candidates > 1 {
			if t.index() < rangeEnd {
			search:
				for {
					w1 := &words[(wordsFound+1)%seaLookahead]
					if w1.candidates(t, e.dict, rangeEnd) > 0 {
						w.markCurrent()
						if t.index() >= rangeEnd {
							break search
						}
						for {
							if words[(wordsFound+2)%seaLookahead].candidates(t, e.dict, rangeEnd) > 0 {
								w.markCurrent()
								break search
							}
							if !w1.backUp(t) {
								break
							}
						}
					}
					if !w.backUp(t) {
						break
					}
				}
			}
			cuWordLength = w.acceptMarked(t)
			cpWordLength = w.markedCPLength()
			wordsFound++
		}
		// A non-word after a short word, not much like a dictionary word
		// itself, is joined to it, up to where a word may begin.
		var uc rune
		if t.index() < rangeEnd && cpWordLength < seaRootCombine {
			wn := &words[wordsFound%seaLookahead]
			if wn.candidates(t, e.dict, rangeEnd) <= 0 &&
				(cuWordLength == 0 || wn.prefix < seaPrefixCombine) {
				remaining := rangeEnd - (current + cuWordLength)
				chars := 0
				for {
					pcIndex := t.index()
					pc := t.next32()
					pcSize := t.index() - pcIndex
					chars += pcSize
					remaining -= pcSize
					if remaining <= 0 {
						break
					}
					uc = t.current32()
					if e.endWord.contains(pc) && e.beginWord.contains(uc) {
						n := words[(wordsFound+1)%seaLookahead].candidates(t, e.dict, rangeEnd)
						t.setIndex(current + cuWordLength + chars)
						if n > 0 {
							break
						}
					}
				}
				if cuWordLength <= 0 {
					wordsFound++
				}
				cuWordLength += chars
			} else {
				t.setIndex(current + cuWordLength)
			}
		}
		// Never stop before a combining mark.
		for {
			pos := t.index()
			if pos >= rangeEnd || !e.marks.contains(t.current32()) {
				break
			}
			t.next32()
			cuWordLength += t.index() - pos
		}
		if e.suffixes && t.index() < rangeEnd && cuWordLength > 0 {
			wn := &words[wordsFound%seaLookahead]
			if wn.candidates(t, e.dict, rangeEnd) <= 0 {
				uc = t.current32()
				if uc == thaiPaiyannoi || uc == thaiMaiyamok {
					if uc == thaiPaiyannoi {
						if p := t.previous32(); p != thaiPaiyannoi && p != thaiMaiyamok {
							t.next32()
							at := t.index()
							t.next32()
							cuWordLength += t.index() - at
							uc = t.current32()
						} else {
							t.next32()
						}
					}
					if uc == thaiMaiyamok {
						if t.previous32() != thaiMaiyamok {
							t.next32()
							at := t.index()
							t.next32()
							cuWordLength += t.index() - at
						} else {
							t.next32()
						}
					}
				} else {
					t.setIndex(current + cuWordLength)
				}
			} else {
				t.setIndex(current + cuWordLength)
			}
		}
		if cuWordLength > 0 {
			*breaks = append(*breaks, current+cuWordLength)
		}
	}
	// No break at the end of the run.
	if len(*breaks) > 0 && peek(*breaks) >= rangeEnd {
		*breaks = (*breaks)[:len(*breaks)-1]
		wordsFound--
	} else if len(*breaks) == 0 && rangeEnd <= 0 {
		wordsFound--
	}
	return wordsFound
}

// cjkEngine is CjkBreakEngine for Chinese and Japanese: the segmentation
// of least cost, each word costing what the dictionary says, and a run of
// katakana what its length does.
type cjkEngine struct {
	dict *breakDictionary
	set  codeRanges
	norm *Normalizer
}

func newCJKEngine(d *breakDictionary, set codeRanges, norm *Normalizer) *cjkEngine {
	return &cjkEngine{dict: d, set: set, norm: norm}
}

func (e *cjkEngine) handles(c rune) bool { return e.set.contains(c) }

func (e *cjkEngine) findBreaks(t *utext, start, end int, breaks *[]int) int {
	return dictionaryFindBreaks(t, start, end, breaks, e.set, e.divide)
}

// hasBoundaryBefore is NFKC's hasBoundaryBefore: nothing before the
// character composes with it or with what it decomposes to.
func (e *cjkEngine) hasBoundaryBefore(c rune) bool {
	first := c
	for {
		if first >= hangulBase && first < hangulBase+hangulSCount {
			first = hangulLBase
			break
		}
		d, ok := e.norm.tables.DecomposeCompatibility(first)
		if !ok || d == "" {
			break
		}
		r := []rune(d)[0]
		if r == first {
			break
		}
		first = r
	}
	if e.norm.tables.Class(first) != 0 {
		return false
	}
	if first >= hangulVBase && first < hangulVBase+hangulVCount ||
		first > hangulTBase && first < hangulTBase+hangulTCount {
		return false
	}
	return !e.norm.tables.CombinesBack(first)
}

const (
	cjkMaxKatakanaLength      = 8
	cjkMaxKatakanaGroupLength = 20
	cjkMaxSnlp                = 255
	cjkMaxWordSize            = 20
)

var katakanaCost = [cjkMaxKatakanaLength + 1]uint32{8192, 984, 408, 240, 204, 252, 300, 372, 480}

func getKatakanaCost(n int) uint32 {
	if n > cjkMaxKatakanaLength {
		return 8192
	}
	return katakanaCost[n]
}

func isKatakana(c rune) bool {
	return c >= 0x30a1 && c <= 0x30fe && c != 0x30fb || c >= 0xff66 && c <= 0xff9f
}

func (e *cjkEngine) divide(t *utext, rangeStart, rangeEnd int, breaks *[]int) int {
	if rangeStart >= rangeEnd {
		return 0
	}
	in := t.text[rangeStart:rangeEnd]
	var inputMap []int // nil for positions offset by rangeStart
	s := string(utf16.Decode(in))
	if e.norm.Normalize(s, NFKC) != s {
		var normalized []uint16
		var nmap []int
		u := utext{text: in}
		for u.index() < len(in) {
			fragStart := u.index()
			var frag []rune
			c := u.next32()
			for {
				frag = append(frag, c)
				if u.index() == len(in) {
					break
				}
				c = u.current32()
				if e.hasBoundaryBefore(c) {
					break
				}
				u.next32()
			}
			nf := utf16.Encode([]rune(e.norm.Normalize(string(frag), NFKC)))
			normalized = append(normalized, nf...)
			for len(nmap) < len(normalized) {
				nmap = append(nmap, fragStart+rangeStart)
			}
		}
		nmap = append(nmap, len(in)+rangeStart)
		inputMap, in = nmap, normalized
	}
	runes := utf16.Decode(in)
	numCodePts := len(runes)
	if numCodePts != len(in) {
		// Supplementary characters: the dictionary counts code points.
		var cp []int
		cu := 0
		for i := 0; ; i++ {
			if inputMap != nil {
				cp = append(cp, inputMap[cu])
			} else {
				cp = append(cp, cu+rangeStart)
			}
			if cu == len(in) {
				break
			}
			if isLead(in[cu]) && cu+1 < len(in) && isTrail(in[cu+1]) {
				cu += 2
			} else {
				cu++
			}
		}
		inputMap = cp
	}
	const maxUint32 = ^uint32(0)
	bestSnlp := make([]uint32, numCodePts+1)
	prev := make([]int, numCodePts+1)
	for i := range bestSnlp {
		bestSnlp[i], prev[i] = maxUint32, -1
	}
	bestSnlp[0] = 0
	values := make([]int, numCodePts+1)
	lengths := make([]int, numCodePts+1)
	fu := utext{text: in}
	ix := 0
	prevKatakana := false
	for i := 0; i < numCodePts; i++ {
		c, n := fu.at(ix)
		if bestSnlp[i] != maxUint32 {
			fu.setIndex(ix)
			count, _ := e.dict.matches(&fu, cjkMaxWordSize, numCodePts, nil, lengths[:numCodePts], values[:numCodePts])
			// A character that is no one-character word is one anyway, at
			// the highest cost, but for Hangul, which stays together.
			if (count == 0 || lengths[0] != 1) && !(c >= 0xac00 && c <= 0xd7a3) {
				values[count] = cjkMaxSnlp
				lengths[count] = 1
				count++
			}
			for j := 0; j < count; j++ {
				snlp := bestSnlp[i] + uint32(values[j])
				if k := lengths[j] + i; snlp < bestSnlp[k] {
					bestSnlp[k] = snlp
					prev[k] = i
				}
			}
			katakana := isKatakana(c)
			if !prevKatakana && katakana {
				run := 1
				j := ix + n
				for j < len(in) && run < cjkMaxKatakanaGroupLength {
					cj, m := fu.at(j)
					if !isKatakana(cj) {
						break
					}
					j += m
					run++
				}
				if run < cjkMaxKatakanaGroupLength {
					snlp := bestSnlp[i] + getKatakanaCost(run)
					if snlp < bestSnlp[i+run] {
						bestSnlp[i+run] = snlp
						prev[i+run] = i
					}
				}
			}
			prevKatakana = katakana
		}
		ix += n
	}
	var tb []int
	if bestSnlp[numCodePts] == maxUint32 {
		tb = append(tb, numCodePts)
	} else {
		for i := numCodePts; i > 0; i = prev[i] {
			tb = append(tb, i)
		}
	}
	if len(*breaks) == 0 || peek(*breaks) < rangeStart {
		tb = append(tb, 0)
	}
	numBreaks := len(tb)
	prevUTextPos := -1
	corrected := 0
	for i := numBreaks - 1; i >= 0; i-- {
		pos := tb[i] + rangeStart
		if inputMap != nil {
			pos = inputMap[tb[i]]
		}
		if pos > prevUTextPos {
			if pos != rangeStart {
				*breaks = append(*breaks, pos)
				corrected++
			}
		}
		prevUTextPos = pos
	}
	if len(*breaks) > 0 && peek(*breaks) == rangeEnd {
		*breaks = (*breaks)[:len(*breaks)-1]
		corrected--
	}
	return corrected
}
