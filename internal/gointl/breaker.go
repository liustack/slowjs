package intl

// ICU's RuleBasedBreakIterator, forward: the rules' state machine
// (handleNext), and the break cache that hands runs of dictionary
// characters to a language's break engine (rbbi_cache.cpp).

import "sort"

// A utext is a UText over UTF-16: a position that stays on code point
// boundaries, and the text read a code point at a time, a lone surrogate
// being a code point of its own.
type utext struct {
	text []uint16
	pos  int
}

func isLead(u uint16) bool  { return u >= 0xd800 && u <= 0xdbff }
func isTrail(u uint16) bool { return u >= 0xdc00 && u <= 0xdfff }

// setIndex is utext_setNativeIndex: pinned to the text, and moved back to
// the start of a surrogate pair.
func (t *utext) setIndex(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(t.text) {
		i = len(t.text)
	}
	if i > 0 && i < len(t.text) && isTrail(t.text[i]) && isLead(t.text[i-1]) {
		i--
	}
	t.pos = i
}

func (t *utext) index() int { return t.pos }

// at is the code point at a position and its length in units; -1 at the
// end.
func (t *utext) at(i int) (rune, int) {
	if i < 0 || i >= len(t.text) {
		return -1, 0
	}
	u := t.text[i]
	if isLead(u) && i+1 < len(t.text) && isTrail(t.text[i+1]) {
		return 0x10000 + (rune(u)-0xd800)<<10 + (rune(t.text[i+1]) - 0xdc00), 2
	}
	return rune(u), 1
}

// next32 is utext_next32: the code point at the position, then past it.
func (t *utext) next32() rune {
	c, n := t.at(t.pos)
	t.pos += n
	return c
}

func (t *utext) current32() rune {
	c, _ := t.at(t.pos)
	return c
}

// previous32 is utext_previous32: back over a code point, which it
// returns; -1 at the start.
func (t *utext) previous32() rune {
	if t.pos <= 0 {
		return -1
	}
	t.pos--
	if t.pos > 0 && isTrail(t.text[t.pos]) && isLead(t.text[t.pos-1]) {
		t.pos--
	}
	c, _ := t.at(t.pos)
	return c
}

// moveIndex32 is utext_moveIndex32 forward.
func (t *utext) moveIndex32(n int) {
	for ; n > 0 && t.pos < len(t.text); n-- {
		t.next32()
	}
}

// A boundary is a break position and the index of its rule status group.
type boundary struct {
	pos, status int
}

// breaker finds a text's boundaries as one RuleBasedBreakIterator
// iterating forward from its start.
type breaker struct {
	rules   *breakRules
	engines *breakEngines
	t       utext

	// handleNext's results besides the position.
	ruleStatus int
	dictCount  int
	lookAhead  []int

	dict dictionaryCache
	// state is the iterator's own engines: those it has been handed, and
	// the scripts it has found none for.
	stack     []breakEngine
	unhandled map[string]bool
}

// dictionaryCache is RuleBasedBreakIterator::DictionaryCache.
type dictionaryCache struct {
	breaks      []int
	start       int
	limit       int
	otherStatus int
}

func (d *dictionaryCache) reset() {
	d.breaks = d.breaks[:0]
	d.start, d.limit = 0, 0
}

// following is DictionaryCache::following: the dictionary boundary after a
// position within the cached range. The breaks ascend, and are searched
// rather than walked: a run of Thai or Chinese is one range, asked once for
// each of its breaks.
func (d *dictionaryCache) following(from int) (boundary, bool) {
	if from >= d.limit || from < d.start {
		return boundary{}, false
	}
	if i := sort.SearchInts(d.breaks, from+1); i < len(d.breaks) {
		return boundary{d.breaks[i], d.otherStatus}, true
	}
	return boundary{}, false
}

// boundaries are every boundary of a text, from 0 to its length.
func (b *breaker) boundaries() []boundary {
	out := []boundary{{0, 0}}
	// ICU leaves the look-ahead results unset: the rules record each before
	// they complete it.
	b.lookAhead = make([]int, b.rules.forward.lookAheadSize)
	for i := range b.lookAhead {
		b.lookAhead[i] = -1
	}
	cur := out[0]
	for {
		if next, ok := b.dict.following(cur.pos); ok {
			out = append(out, next)
			cur = next
			continue
		}
		pos, done := b.handleNext(cur.pos)
		if done {
			break
		}
		next := boundary{pos, b.ruleStatus}
		if b.dictCount > 0 {
			b.populateDictionary(cur.pos, pos, cur.status, b.ruleStatus)
			if d, ok := b.dict.following(cur.pos); ok {
				next = d
			}
		}
		if next.pos <= cur.pos || next.pos > len(b.t.text) {
			// Rules or a dictionary a Source corrupted, which went back or
			// past the end: on by a code point, as a match of nothing is.
			b.t.setIndex(cur.pos)
			b.t.next32()
			next = boundary{b.t.index(), 0}
			if next.pos <= cur.pos {
				break
			}
		}
		out = append(out, next)
		cur = next
	}
	return out
}

// handleNext is RuleBasedBreakIterator::handleNext: the rules' state
// machine run from a position to the next boundary.
func (b *breaker) handleNext(from int) (int, bool) {
	const (
		stopState  = 0
		startState = 1
		modeRun    = iota
		modeStart
		modeEnd
	)
	table := &b.rules.forward
	b.ruleStatus = 0
	b.dictCount = 0
	b.t.setIndex(from)
	initial := b.t.index()
	result := initial
	c := b.t.next32()
	if c < 0 {
		return 0, true
	}
	state := startState
	mode := modeRun
	category := 0
	if table.flags&rbbiBOFRequired != 0 {
		category = 2
		mode = modeStart
	}
	for {
		if c < 0 {
			if mode == modeEnd {
				break
			}
			mode = modeEnd
			category = 1
		}
		if mode == modeRun {
			category = b.rules.trie.get(c)
			if category >= table.dictStart {
				b.dictCount++
			}
		}
		state = table.next(state, category)
		if accepting := table.accepting(state); accepting == 1 {
			if mode != modeStart {
				result = b.t.index()
			}
			b.ruleStatus = table.tagsIdx(state)
		} else if accepting > 1 {
			if r := b.lookAhead[accepting]; r >= 0 {
				b.ruleStatus = table.tagsIdx(state)
				return r, false
			}
		}
		if rule := table.lookAhead(state); rule > 1 {
			b.lookAhead[rule] = b.t.index()
		}
		if state == stopState {
			break
		}
		if mode == modeRun {
			c = b.t.next32()
		} else if mode == modeStart {
			mode = modeRun
		}
	}
	// A match that advanced nothing is forced on by a code point.
	if result == initial {
		b.t.setIndex(initial)
		b.t.next32()
		result = b.t.index()
		b.ruleStatus = 0
	}
	return result, false
}

// populateDictionary is DictionaryCache::populateDictionary: the runs of
// dictionary characters between two rule boundaries, each handed to the
// engine for its first character.
func (b *breaker) populateDictionary(start, end, firstStatus, otherStatus int) {
	if end-start <= 1 {
		return
	}
	b.dict.reset()
	b.dict.otherStatus = otherStatus
	found := 0
	t := &b.t
	t.setIndex(start)
	c := t.current32()
	category := b.rules.trie.get(c)
	dictStart := b.rules.forward.dictStart
	for {
		current := t.index()
		for current < end && category < dictStart {
			t.next32()
			c = t.current32()
			category = b.rules.trie.get(c)
			current = t.index()
		}
		if current >= end {
			break
		}
		if e := b.engineFor(c); e != nil {
			found += e.findBreaks(t, current, end, &b.dict.breaks)
		}
		if t.index() <= current {
			// An engine reading a corrupt dictionary, which did not move:
			// on by a code point, rather than asking it again for ever.
			t.setIndex(current)
			t.next32()
		}
		c = t.current32()
		category = b.rules.trie.get(c)
	}
	if found > 0 {
		if start < b.dict.breaks[0] {
			b.dict.breaks = append([]int{start}, b.dict.breaks...)
		}
		if end > b.dict.breaks[len(b.dict.breaks)-1] {
			b.dict.breaks = append(b.dict.breaks, end)
		}
		b.dict.start = b.dict.breaks[0]
		b.dict.limit = b.dict.breaks[len(b.dict.breaks)-1]
	}
}

// engineFor is getLanguageBreakEngine: the iterator's engines, latest
// first, then the factory's, then the engine for characters without one.
//
// ICU's factory keeps, for the whole process, every engine it has made,
// and one made for any character claims every character in its set. This
// reckons as a process that has made them all, as any process that has
// broken text in those scripts has; before that, a character such as
// U+30FC, of the Common script but in the Chinese and Japanese engine's
// set, finds no engine when it comes first.
func (b *breaker) engineFor(c rune) breakEngine {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if b.stack[i].handles(c) {
			return b.stack[i]
		}
	}
	if e := b.engines.forChar(c); e != nil {
		b.stack = append(b.stack, e)
		return e
	}
	script := b.engines.script(c)
	if b.unhandled == nil {
		b.unhandled = map[string]bool{}
		// It goes at the bottom of the stack, to be tried last.
		b.stack = append([]breakEngine{unhandledEngine{b}}, b.stack...)
	}
	b.unhandled[script] = true
	return b.stack[0]
}

// unhandledEngine is UnhandledEngine: it skips the characters of the
// scripts it has been handed, breaking nothing.
type unhandledEngine struct{ b *breaker }

func (u unhandledEngine) handles(c rune) bool { return u.b.unhandled[u.b.engines.script(c)] }

func (u unhandledEngine) findBreaks(t *utext, start, end int, breaks *[]int) int {
	t.setIndex(start)
	c := t.current32()
	for t.index() < end && u.handles(c) {
		t.next32()
		c = t.current32()
	}
	return 0
}
