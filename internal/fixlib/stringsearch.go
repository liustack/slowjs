// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// searchFuncs are the functions whose loops call stringFind of
// lib/stringsearch.go, and the ones stringFind repeats, without the change
// of fixStringSearch.
var searchFuncs = []string{"_js_string_includes", "_js_string_indexOf", "_string_cmp", "_string_get"}

// The loop of String.prototype.includes, startsWith and endsWith, and of
// indexOf and lastIndexOf, which compares the string v (p11) with the
// string (p1) at each index from start to stop. Group 2 is what the index
// moves by.
var reSearchLoop = regexp.MustCompile(`\n\t\ti = start\n\t\tfor \{\n(\t\t\tif !\(_string_cmp\(tls, p1, p11, i, 0, v_len\) != 0\) \{\n\t\t\t\tret = (?:int32\(1\)|i)\n\t\t\t\tbreak\n\t\t\t\}\n\t\t\tif i == stop \{\n\t\t\t\tbreak\n\t\t\t\}\n\t\t\tgoto _\d+\n\t\t_\d+:\n\t\t\t;\n\t\t\ti = i \+ (1|inc)\n\t\t\}\n)`)

const searchMark = "\t\t\t// Changed by internal/fixlib: skip to where the string is, in one search (see fixStringSearch).\n"

// searchCall is the call fixStringSearch puts at the start of the loop,
// which looks for p11 in p1 from i to stop by step inc and counts the
// indexes it looks through on the context, which ccgo names ctx2 in both
// functions (searchSites pins their text, the parameters included).
func searchCall(inc string) string {
	return searchMark + "\t\t\tif i = stringFind(ctx2, p1, p11, i, stop, " + inc + "); i < 0 {\n\t\t\t\tbreak\n\t\t\t}\n"
}

// The calls fixStringSearch writes, in js_string_includes and in
// js_string_indexOf.
var searchCalls = []string{searchCall("int32(1)"), searchCall("inc")}

// fixStringSearch makes the loops that look for a string in a string by
// comparing the whole of it at one index after the other call stringFind
// of lib/stringsearch.go at the start of each turn: those of
// js_string_includes (includes, startsWith, endsWith) and js_string_indexOf
// (indexOf, lastIndexOf). stringFind returns the next index where the
// string is, found with the search of package bytes, and the loop compares
// there as before and ends, or it returns -1 and the loop ends without a
// match. stringFind reads a JSString with the constants of fixJSONStrings.
// string_indexof (split, replace, replaceAll) is left as it is: it already
// skips to the next index of the first character, and its strings are
// mostly short, where a call of stringFind costs more than it saves.
//
// stringFind repeats what string_cmp and string_get compare, so the text
// of searchFuncs is pinned in searchSites: when a release changes one of
// them, fixStringSearch stops. It starts from the text without its change
// (restoreStringSearch), and checks that restoreStringSearch gives that
// text back.
func fixStringSearch(b []byte, file string) ([]byte, error) {
	plain, err := restoreStringSearch(b)
	if err != nil {
		return nil, err
	}
	out, got, err := planStringSearch(plain)
	if err != nil {
		return nil, err
	}
	if want := searchSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/stringsearch.go, then update searchSites (go run ./internal/fixlib -search)", file, strings.Join(searchFuncs, ", "), got, want)
	}
	back, err := restoreStringSearch(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, plain) {
		return nil, fmt.Errorf("fixStringSearch: restoreStringSearch does not give the input back")
	}
	return out, nil
}

// planStringSearch returns b, which has no change of fixStringSearch, with
// the change, and the fingerprint of searchFuncs.
func planStringSearch(b []byte) ([]byte, string, error) {
	got, err := funcsFingerprint(b, searchFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return nil, "", err
	}
	out := b
	for k, c := range []struct{ name, step string }{
		{"_js_string_includes", "1"},
		{"_js_string_indexOf", "inc"},
	} {
		out, _, err = jsonInsert(out, c.name, reSearchLoop, func(m [][]byte) (string, error) {
			if string(m[2]) != c.step {
				return "", fmt.Errorf("%s: the loop moves the index by %s, want %s", c.name, m[2], c.step)
			}
			return searchCalls[k], nil
		})
		if err != nil {
			return nil, "", err
		}
	}
	return out, got, nil
}

// restoreStringSearch undoes fixStringSearch.
func restoreStringSearch(b []byte) ([]byte, error) {
	out := b
	for _, call := range searchCalls {
		out = bytes.Replace(out, []byte(call), nil, 1)
	}
	if bytes.Contains(out, []byte("stringFind(")) {
		return nil, fmt.Errorf("restoreStringSearch: a call of stringFind is left that is not what fixStringSearch writes")
	}
	return out, nil
}

// searchFingerprint is the fingerprint fixStringSearch checks, for -search.
func searchFingerprint(b []byte) (string, error) {
	plain, err := restoreStringSearch(b)
	if err != nil {
		return "", err
	}
	_, fp, err := planStringSearch(plain)
	return fp, err
}
