// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package t262 reads the metadata of test262 tests and decides how each one
// runs, the same way for every engine.
package t262

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Meta is the front matter of a test.
type Meta struct {
	Includes    []string
	Flags       []string
	Features    []string
	NegPhase    string
	NegType     string
	HasNegative bool
}

func (m *Meta) Flag(f string) bool {
	for _, x := range m.Flags {
		if x == f {
			return true
		}
	}
	return false
}

var fmRE = regexp.MustCompile(`(?s)/\*---(.*?)---\*/`)

// Parse reads the front matter of a test source.
func Parse(src string) *Meta {
	m := &Meta{}
	sm := fmRE.FindStringSubmatch(src)
	if sm == nil {
		return m
	}
	lines := strings.Split(sm[1], "\n")
	key := ""
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		indented := len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t')
		if !indented && strings.Contains(t, ":") {
			i := strings.Index(t, ":")
			key = strings.TrimSpace(t[:i])
			val := strings.TrimSpace(t[i+1:])
			switch key {
			case "includes":
				m.Includes = append(m.Includes, inline(val)...)
			case "flags":
				m.Flags = append(m.Flags, inline(val)...)
			case "features":
				m.Features = append(m.Features, inline(val)...)
			case "negative":
				m.HasNegative = true
			}
			continue
		}
		if !indented {
			key = ""
			continue
		}
		switch key {
		case "includes", "flags", "features":
			if strings.HasPrefix(t, "- ") {
				v := strings.TrimSpace(t[2:])
				switch key {
				case "includes":
					m.Includes = append(m.Includes, v)
				case "flags":
					m.Flags = append(m.Flags, v)
				case "features":
					m.Features = append(m.Features, v)
				}
			}
		case "negative":
			if strings.HasPrefix(t, "phase:") {
				m.NegPhase = strings.TrimSpace(strings.TrimPrefix(t, "phase:"))
			}
			if strings.HasPrefix(t, "type:") {
				m.NegType = strings.TrimSpace(strings.TrimPrefix(t, "type:"))
			}
		}
	}
	return m
}

func inline(v string) []string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") {
		return nil
	}
	v = strings.Trim(v, "[]")
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

var hostHookRE = regexp.MustCompile(`\$262\.(createRealm|detachArrayBuffer|evalScript|agent|gc|IsHTMLDDA|AbstractModuleSource)`)

// Plan says how a test runs: the modes ("sloppy", "strict", "raw"), or why
// it is left out.
type Plan struct {
	Path    string
	Modes   []string
	Exclude string // "module", "agent", "host-hook"; empty when it runs
}

// Dirs are the parts of test262 the comparison runs.
var Dirs = []string{"test/language", "test/built-ins", "test/intl402"}

// Collect plans every test under root's Dirs.
func Collect(root string) ([]Plan, error) {
	harness := map[string]string{}
	readHarness := func(name string) string {
		if s, ok := harness[name]; ok {
			return s
		}
		b, _ := os.ReadFile(filepath.Join(root, "harness", name))
		harness[name] = string(b)
		return harness[name]
	}
	var plans []Plan
	for _, d := range Dirs {
		err := filepath.WalkDir(filepath.Join(root, d), func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if de.IsDir() || !strings.HasSuffix(p, ".js") || strings.Contains(p, "_FIXTURE") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			src := string(b)
			m := Parse(src)
			rel, _ := filepath.Rel(root, p)
			pl := Plan{Path: filepath.ToSlash(rel)}
			text := src
			for _, inc := range m.Includes {
				text += readHarness(inc)
			}
			switch {
			case m.Flag("module"):
				pl.Exclude = "module"
			case m.Flag("CanBlockIsFalse") || m.Flag("CanBlockIsTrue"):
				pl.Exclude = "agent"
			case hostHookRE.MatchString(text):
				pl.Exclude = "host-hook"
			case m.Flag("raw"):
				pl.Modes = []string{"raw"}
			case m.Flag("onlyStrict"):
				pl.Modes = []string{"strict"}
			case m.Flag("noStrict"):
				pl.Modes = []string{"sloppy"}
			default:
				pl.Modes = []string{"sloppy", "strict"}
			}
			plans = append(plans, pl)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Path < plans[j].Path })
	return plans, nil
}

// Area is the directory a test counts under in the results:
// test/built-ins/RegExp/... is "built-ins/RegExp".
func Area(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "test/"), "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// Source builds the script a test runs as in mode.
func Source(root, path, mode string, harness func(string) string) (string, *Meta, error) {
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return "", nil, err
	}
	src := string(b)
	m := Parse(src)
	if mode == "raw" {
		return src, m, nil
	}
	var sb strings.Builder
	if mode == "strict" {
		sb.WriteString("\"use strict\";\n")
	}
	sb.WriteString(harness("assert.js"))
	sb.WriteString("\n")
	sb.WriteString(harness("sta.js"))
	sb.WriteString("\n")
	if m.Flag("async") {
		sb.WriteString(harness("doneprintHandle.js"))
		sb.WriteString("\n")
	}
	for _, inc := range m.Includes {
		sb.WriteString(harness(inc))
		sb.WriteString("\n")
	}
	sb.WriteString(src)
	return sb.String(), m, nil
}

// Newer lists the test262 feature tags of ECMAScript 2024 and later, and of
// proposals not yet in a published edition (from features.txt). A test with
// none of them tests ES2023 or older, the edition QuickJS says it supports.
var Newer = map[string]bool{}

func init() {
	for _, f := range strings.Fields(`
array-grouping arraybuffer-transfer promise-with-resolvers regexp-v-flag resizable-arraybuffer
String.prototype.isWellFormed String.prototype.toWellFormed Atomics.waitAsync
iterator-helpers set-methods Float16Array RegExp.escape regexp-modifiers json-modules import-attributes
promise-try regexp-duplicate-named-groups
Error.isError Math.sumPrecise uint8array-base64 explicit-resource-management Array.fromAsync Atomics.pause
upsert iterator-sequencing json-parse-with-source Temporal iterator-chunking iterator-includes joint-iteration
canonical-tz Intl.DurationFormat Intl-enumeration Intl.DisplayNames-v2
Intl.Era-monthcode Intl.Locale-info Intl.NumberFormat-v3 legacy-regexp ShadowRealm decorators
source-phase-imports source-phase-imports-module-source import-defer export-defer import-text
immutable-arraybuffer import-bytes nonextensible-applies-to-private await-dictionary error-stack-accessor
Iterator.prototype.join`) {
		Newer[f] = true
	}
}

// IsNewer reports whether a test uses a feature newer than ES2023.
func IsNewer(features []string) bool {
	for _, f := range features {
		if Newer[f] {
			return true
		}
	}
	return false
}
