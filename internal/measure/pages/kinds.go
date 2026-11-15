// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package pages

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Kind is a set of pages of the repository: a bundle and the inputs of its pages, in the
// order they render.
type Kind struct {
	Bundle string   // path from the root of the repository
	Inputs []string // the same
	React  bool     // the bundle defines render(), which returns the HTML, instead of startRender
}

// kindGlobs are the kinds of the timings: the bundle and the inputs, which a glob may name.
var kindGlobs = map[string]struct {
	bundle string
	inputs string
	react  bool
}{
	// the six pages of the workbench of tifa's example shop
	"wb": {"engine/pgo/pages/shop/shop.js", "engine/pgo/pages/shop/in/wb-*.json", false},
	// its fourteen store pages, in en-US and zh-CN
	"store": {"engine/pgo/pages/shop/shop.js", "engine/pgo/pages/shop/in/store-*.json", false},
	// the three pages of tifa's MUI test app
	"mui": {"engine/pgo/pages/mui/mui.js", "engine/pgo/pages/mui/in/*.json", false},
	// React's page of compare/engines, one render() with no input
	"react": {"compare/engines/react/react-ssr.js", "", true},
}

// Kinds returns the names of the kinds, sorted.
func Kinds() []string {
	var k []string
	for n := range kindGlobs {
		k = append(k, n)
	}
	slices.Sort(k)
	return k
}

// FindKind returns the kind named name in the repository repo. pages, when not empty, keeps only
// the pages of those names (the names of their inputs without .json), in that order.
func FindKind(repo, name string, pages []string) (Kind, error) {
	g, ok := kindGlobs[name]
	if !ok {
		return Kind{}, fmt.Errorf("pages: no kind %q, the kinds are %s", name, strings.Join(Kinds(), ", "))
	}
	k := Kind{Bundle: filepath.Join(repo, filepath.FromSlash(g.bundle)), React: g.react}
	if g.inputs == "" {
		if len(pages) > 0 {
			return Kind{}, fmt.Errorf("pages: kind %s has no inputs to choose from", name)
		}
		return k, nil
	}
	all, err := filepath.Glob(filepath.Join(repo, filepath.FromSlash(g.inputs)))
	if err != nil {
		return Kind{}, err
	}
	if len(all) == 0 {
		return Kind{}, fmt.Errorf("pages: no input %s in %s", g.inputs, repo)
	}
	slices.Sort(all)
	if len(pages) == 0 {
		k.Inputs = all
		return k, nil
	}
	for _, p := range pages {
		i := slices.IndexFunc(all, func(f string) bool { return PageName(f) == p })
		if i < 0 {
			return Kind{}, fmt.Errorf("pages: kind %s has no page %q", name, p)
		}
		k.Inputs = append(k.Inputs, all[i])
	}
	return k, nil
}

// PageName is the name of the page of an input: its file name without .json.
func PageName(input string) string {
	return strings.TrimSuffix(filepath.Base(input), ".json")
}

// Repo returns the root of the liustack/slowjs checkout: dir when it is not empty, else the
// first folder from the working directory up whose go.mod is the module of slowjs.
func Repo(dir string) (string, error) {
	if dir != "" {
		return filepath.Abs(dir)
	}
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/liustack/slowjs\n")) {
			return d, nil
		}
		up := filepath.Dir(d)
		if up == d {
			return "", fmt.Errorf("pages: no liustack/slowjs checkout above the working directory: pass -repo")
		}
		d = up
	}
}
