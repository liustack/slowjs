// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fixRegexSkip makes lre_exec call lreSkip of lib/regexskip.go first, which
// skips the positions of the string where the regexp cannot start to match,
// and run the bytecode through lreRun, which tries the match at each
// position lreSkip and itself find, or return what lreSkip gives when there
// is nothing to run. It also makes the split ops of lre_exec_backtrack ask
// lreSplit, when the way they push starts with a character, a class of
// characters or a save, whether that way can start at the position at all,
// and push nothing when it cannot, counting a step toward the interrupt
// check in place of the pop it saves.
//
// lreSkip, lreRun and lreSplit read the bytecode lre_compile writes and do
// what lre_exec and lre_exec_backtrack do for a few ops, lreFirstSet
// relies on what lre_canonicalize gives for the ASCII characters, and
// lreRun and the split ops count steps toward the interrupt check through
// lre_poll_timeout. So the
// generated text of those functions is pinned in regexSites: when a release
// changes one of them, fixRegexSkip stops, and the new code must be read
// against lib/regexskip.go before the fingerprints are updated (go run
// ./internal/fixlib -regex prints them). The fingerprints are of the text
// without the change, after every other change before it.
func fixRegexSkip(b []byte, file string) ([]byte, error) {
	got, err := regexFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := regexSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/regexskip.go, then update regexSites (go run ./internal/fixlib -regex)", file, "lre_compile, lre_exec, lre_exec_backtrack, lre_canonicalize or lre_poll_timeout", got, want)
	}

	body, err := funcBody(b, reLreExecHead, "Xlre_exec")
	if err != nil {
		return nil, err
	}
	start := bytes.Index(b, body)
	fixed, err := insertAt("Xlre_exec", body, regexSkipCall, func(body []byte) (int, error) {
		return reLreExecHead.FindIndex(body)[1], nil
	})
	if err != nil {
		return nil, err
	}
	switch old, new := bytes.Count(fixed, []byte(regexRunOld)), bytes.Count(fixed, []byte(regexRunNew)); {
	case old == 1 && new == 0:
		fixed = bytes.Replace(fixed, []byte(regexRunOld), []byte(regexRunNew), 1)
	case old == 0 && new == 1:
	default:
		return nil, fmt.Errorf("Xlre_exec: want one call of lre_exec_backtrack from the start of the bytecode, found %d, and %d of lreRun", old, new)
	}
	b = append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...)

	if body, err = funcBody(b, funcHead(regexMachine), regexMachine); err != nil {
		return nil, err
	}
	start = bytes.Index(b, body)
	if fixed, err = regexSplitFirst(body); err != nil {
		return nil, err
	}
	return append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...), nil
}

const regexMachine = "_lre_exec_backtrack"

var (
	reLreExecHead = regexp.MustCompile(`\nfunc Xlre_exec\(tls \*libc\.TLS, capture uintptr, bc_buf uintptr, cbuf uintptr, cindex int32, clen int32, cbuf_type int32, opaque uintptr\) \(r int32\) \{\n`)
	regexFuncs    = []string{"Xlre_compile", "Xlre_exec", regexMachine, "Xlre_canonicalize", "_lre_case_folding_entry", "_lre_poll_timeout"}

	// The two ways of a split op of lre_exec_backtrack, as ccgo writes
	// them: pc, the way it takes now, and pc1, the way it pushes. The
	// conversion of the offset differs between targets.
	reRegexSplit = regexp.MustCompile(`\t\tif opcode == int32\(EREOP_split_next_first\) \{\n\t\t\tpc1 = pc \+ uintptr\(([^\n]+)\)\n\t\t\} else \{\n\t\t\tpc1 = pc\n\t\t\tpc = pc \+ uintptr\(([^\n]+)\)\n\t\t\}\n`)
	// The push of pc1 and the position by a split op, after the check of
	// the room on the stack, and its jump to the next op.
	reRegexSplitPush = regexp.MustCompile(`^(?:\t\t[^\n]*\n)*?\t\t\*\(\*uintptr\)\(unsafe\.Pointer\(sp\)\) = pc1\n\t\t\*\(\*uintptr\)\(unsafe\.Pointer\(sp \+ 1\*(?:4|8)\)\) = cptr\n(?:\t\t[^\n]*\n){2}\t\tsp = sp \+ uintptr\(3\)\*(?:4|8)\n\t\tbp = sp\n\t\tgoto (_\d+)\n`)
	// The jump to the next op after the switch of the opcode.
	reRegexNext = regexp.MustCompile(`\n\t\tdefault:\n\t\t\tgoto _\d+\n\t\t\}\n\t\tgoto (_\d+)\n`)
	// The question to lreSplit, which fixRegexSkip puts after the two ways.
	reRegexSplitHook = regexp.MustCompile(regexp.QuoteMeta(regexSplitComment) + `\t\tif lrePeekOps>>\*\(\*uint8\)\(unsafe\.Pointer\(pc1\)\)&1 != 0 \{\n\t\t\tif pc, pc1 = lreSplit\(tls, s, pc, pc1, cptr\); pc1 == 0 \{\n\t\t\t\tif _lre_poll_timeout\(tls, s\) != 0 \{\n\t\t\t\t\treturn Tintptr_t\(MLRE_RET_TIMEOUT\)\n\t\t\t\t\}\n\t\t\t\tgoto (_\d+)\n\t\t\t\}\n\t\t\}\n`)
)

const (
	regexSkipCall = "\t// Changed by internal/fixlib: start where a match can start (see fixRegexSkip).\n" +
		"\tskip := lreSkip(tls, capture, bc_buf, cbuf, cindex, clen, cbuf_type, opaque)\n" +
		"\tif skip.pc == 0 {\n\t\treturn skip.index\n\t}\n" +
		"\tcindex = skip.index\n"
	regexRunOld       = "_lre_exec_backtrack(tls, s, capture, bc_buf+uintptr(MRE_HEADER_LEN), cptr)"
	regexRunNew       = "lreRun(tls, s, capture, bc_buf, cptr, &skip)"
	regexSplitComment = "\t\t// Changed by internal/fixlib: a way that cannot start here is not pushed (see fixRegexSkip).\n"
)

// regexSplitHook is the question to lreSplit that fixRegexSkip puts after
// the two ways of a split op, with next the label of the jump to the next
// op. When lreSplit pushes nothing, the hook counts a step toward the
// interrupt check, as the pop of the state it did not push would have:
// the machine checks only on a jump back, a loop and a failed pop, and a
// greedy x+ over a long string makes no pop until it reaches the end.
func regexSplitHook(next string) string {
	return regexSplitComment +
		"\t\tif lrePeekOps>>*(*uint8)(unsafe.Pointer(pc1))&1 != 0 {\n" +
		"\t\t\tif pc, pc1 = lreSplit(tls, s, pc, pc1, cptr); pc1 == 0 {\n" +
		"\t\t\t\tif _lre_poll_timeout(tls, s) != 0 {\n" +
		"\t\t\t\t\treturn Tintptr_t(MLRE_RET_TIMEOUT)\n\t\t\t\t}\n" +
		"\t\t\t\tgoto " + next + "\n\t\t\t}\n\t\t}\n"
}

// regexSplitFirst puts the question to lreSplit after the two ways of the
// split op in body, the text of lre_exec_backtrack, or checks that it is
// there. The op has one handler for both split ops, which pushes pc1 and
// goes on with the next op, at the label the switch of the opcode jumps to
// after each op.
func regexSplitFirst(body []byte) ([]byte, error) {
	hooks := findAllSubmatch(reRegexSplitHook, body)
	bare := reRegexSplitHook.ReplaceAll(body, nil)
	if bytes.Contains(bare, []byte("lreSplit(")) {
		return nil, fmt.Errorf("%s: a question to lreSplit is not the one fixlib writes", regexMachine)
	}
	splits := findAll(reRegexSplit, bare)
	next := findAllSubmatch(reRegexNext, bare)
	if len(splits) != 1 || len(next) != 1 || len(hooks) > 1 {
		return nil, fmt.Errorf("%s: want one handler of the split ops, one jump to the next op after the switch and at most one question to lreSplit, found %d, %d and %d", regexMachine, len(splits), len(next), len(hooks))
	}
	at := splits[0][1]
	push := reRegexSplitPush.FindSubmatch(bare[at:])
	if push == nil || !bytes.Equal(push[1], next[0][1]) {
		return nil, fmt.Errorf("%s: the split ops no longer push pc1 and go on with the next op", regexMachine)
	}
	hook := regexSplitHook(string(next[0][1]))
	if len(hooks) == 1 {
		if !bytes.Equal(hooks[0][0], []byte(hook)) || !bytes.Contains(body, append(bare[splits[0][0]:at], hook...)) {
			return nil, fmt.Errorf("%s: the question to lreSplit is not where fixlib puts it", regexMachine)
		}
		return body, nil
	}
	return splice(bare, at, hook), nil
}

// unfixRegexSkip gives text without the changes of fixRegexSkip.
func unfixRegexSkip(text []byte) []byte {
	text = bytes.Replace(text, []byte(regexSkipCall), nil, 1)
	text = bytes.Replace(text, []byte(regexRunNew), []byte(regexRunOld), 1)
	return reRegexSplitHook.ReplaceAll(text, nil)
}

// regexFingerprint hashes regexFuncs, without the change of fixRegexSkip,
// with each C string written out in place of its offset.
func regexFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, regexFuncs, unfixRegexSkip)
}
