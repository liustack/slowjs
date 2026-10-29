// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command cycles lists the cycles of the engine's call graph that no counted
// depth cuts.
//
//	go run ./internal/goasm/cmd/cycles ENGINE.ll
//
// ENGINE.ll is the engine as one module of LLVM IR, built with -O0 and
// -fsanitize=kcfi (see engine/test/cycles.sh). A function calls the functions
// it names, and an indirect call reaches every function whose address is taken
// and whose kcfi type is that of the call, as the generator joins them. A
// function that calls one of counts (js_call_enter, js_internal_enter,
// js_json_enter and the like, see docs/fork-changes/c-engine.md) is a cut: a
// cycle through it is bounded by its count. It prints each strongly connected
// component left once the cuts are taken out, with more than one function or a
// function that calls itself: its size and its functions, sorted, one per
// line, largest first.
//
// It then prints each function of a noReach pair that the other reaches, with
// a shortest path to it: "reach F G via F A B G". None is expected. The bound
// of a nesting rests on these: JS_ThrowError2 nests twice at most because
// making an error runs no JavaScript and reads no interrupt flag, and
// free_zero_refcount once because freeing starts no collection (see
// js_nest_enter in quickjs.c). A function of a pair that the module does not
// define is an error.
package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// counts: js_json_enter is js_internal_enter with the error of JSON, it counts
// the same level.
var counts = []string{"js_call_enter", "js_internal_enter", "js_internal_enter_rt",
	"lre_depth_enter", "js_nest_enter", "js_json_enter"}

// pair is (F, G): F must not reach G by any path of calls, counted or not.
type pair struct{ from, to string }

var noReach = []pair{
	{"JS_ThrowError2", "JS_CallInternal"},
	{"JS_ThrowError2", "JS_ThrowInterrupted"},
	// Nor does it read the interrupt flag: these are the functions that do
	// (poll_flag), or that the flag is handed to.
	{"JS_ThrowError2", "js_poll_interrupts"},
	{"JS_ThrowError2", "lre_check_timeout"},
	{"JS_ThrowError2", "lre_poll_timeout"},
	{"JS_ThrowError2", "rqsort_stop"},
	// A collection throws nothing, so it starts no error, whose object can
	// start a collection.
	{"JS_RunGC", "JS_ThrowError2"},
	{"JS_RunYoungGC", "JS_ThrowError2"},
	// Freeing starts no collection: free_zero_refcount is never entered
	// twice (JS_NEST_FREE). The finalizers of the host are not in the graph.
	{"free_zero_refcount", "JS_RunGC"},
	{"free_zero_refcount", "JS_RunYoungGC"},
}

const name = `("[^"]+"|[-\w.$]+)`

var (
	defineRE   = regexp.MustCompile(`^define\b.*?@` + name + `\(`)
	callRE     = regexp.MustCompile(`(?:^\s+|=\s+)(?:(?:tail|musttail|notail)\s+)?(?:call|invoke)\s.*?\s([@%])` + name + `\(`)
	kcfiCallRE = regexp.MustCompile(`"kcfi"\(i32 (-?\d+)\)`)
	kcfiTypeRE = regexp.MustCompile(`!kcfi_type !(\d+)`)
	nameRE     = regexp.MustCompile(`@` + name)
	metadataRE = regexp.MustCompile(`^!(\d+) = .*?i32 (-?\d+)`)
)

// graph is the call graph of a module: the functions each function reaches,
// and the functions that count a depth.
type graph struct {
	adj  map[string][]string // sorted
	cuts map[string]bool
}

// parse reads the module text.
func parse(text string) (*graph, error) {
	lines := strings.Split(text, "\n")
	types := map[string]int{}
	for _, line := range lines {
		if m := metadataRE.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[2])
			if err != nil {
				return nil, err
			}
			types[m[1]] = n
		}
	}
	funcs := map[string]*int{} // the kcfi type of each defined function, nil if it has none
	calls := map[string]map[string]bool{}
	icalls := map[string][]int{}
	taken := map[string]bool{}
	take := func(s string) {
		for _, m := range nameRE.FindAllStringSubmatch(s, -1) {
			taken[strings.Trim(m[1], `"`)] = true
		}
	}
	cur := ""
	in := false
	for _, line := range lines {
		if strings.HasPrefix(line, "define") {
			m := defineRE.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("cannot read the name of the function in: %s", line)
			}
			cur, in = strings.Trim(m[1], `"`), true
			var t *int
			if k := kcfiTypeRE.FindStringSubmatch(line); k != nil {
				v, ok := types[k[1]]
				if !ok {
					return nil, fmt.Errorf("the kcfi type !%s of %s has no metadata", k[1], cur)
				}
				t = &v
			}
			funcs[cur] = t
			calls[cur], icalls[cur] = map[string]bool{}, nil
			continue
		}
		if strings.HasPrefix(line, "}") {
			in = false
			continue
		}
		if !in {
			if strings.HasPrefix(line, "@") {
				if _, rhs, ok := strings.Cut(line, "="); ok {
					take(rhs)
				}
			}
			continue
		}
		rest := line
		if m := callRE.FindStringSubmatchIndex(line); m != nil {
			if line[m[2]:m[3]] == "@" {
				calls[cur][strings.Trim(line[m[4]:m[5]], `"`)] = true
				rest = line[:m[4]] + line[m[5]:]
			} else {
				k := kcfiCallRE.FindStringSubmatch(line)
				if k == nil {
					return nil, fmt.Errorf("an indirect call with no kcfi type in %s", cur)
				}
				n, err := strconv.Atoi(k[1])
				if err != nil {
					return nil, err
				}
				icalls[cur] = append(icalls[cur], n)
			}
		}
		take(rest)
	}
	byType := map[int]map[string]bool{}
	for f, t := range funcs {
		if taken[f] && t != nil {
			if byType[*t] == nil {
				byType[*t] = map[string]bool{}
			}
			byType[*t][f] = true
		}
	}
	g := &graph{adj: map[string][]string{}, cuts: map[string]bool{}}
	for f := range funcs {
		s := map[string]bool{}
		for c := range calls[f] {
			if _, ok := funcs[c]; ok {
				s[c] = true
			}
		}
		for _, k := range icalls[f] {
			for h := range byType[k] {
				s[h] = true
			}
		}
		out := make([]string, 0, len(s))
		for h := range s {
			out = append(out, h)
		}
		sort.Strings(out)
		g.adj[f] = out
		for _, c := range counts {
			if calls[f][c] {
				g.cuts[f] = true
			}
		}
	}
	return g, nil
}

// components returns Tarjan's strongly connected components of nodes, without
// recursion.
func (g *graph) components(nodes map[string]bool) [][]string {
	index, low := map[string]int{}, map[string]int{}
	on := map[string]bool{}
	var stack []string
	var out [][]string
	n := 0
	type frame struct {
		v string
		i int
	}
	roots := make([]string, 0, len(nodes))
	for v := range nodes {
		roots = append(roots, v)
	}
	sort.Strings(roots)
	for _, root := range roots {
		if _, ok := index[root]; ok {
			continue
		}
		work := []frame{{root, 0}}
		for len(work) > 0 {
			top := work[len(work)-1]
			work = work[:len(work)-1]
			v, i := top.v, top.i
			if i == 0 {
				index[v], low[v] = n, n
				n++
				stack = append(stack, v)
				on[v] = true
			}
			var succ []string
			for _, w := range g.adj[v] {
				if nodes[w] {
					succ = append(succ, w)
				}
			}
			descended := false
			for i < len(succ) {
				w := succ[i]
				i++
				if _, ok := index[w]; !ok {
					work = append(work, frame{v, i}, frame{w, 0})
					descended = true
					break
				}
				if on[w] {
					low[v] = min(low[v], index[w])
				}
			}
			if descended {
				continue
			}
			if low[v] == index[v] {
				var comp []string
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					on[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				out = append(out, comp)
			}
			if len(work) > 0 {
				u := work[len(work)-1].v
				low[u] = min(low[u], low[v])
			}
		}
	}
	return out
}

// cycles returns the cycles no count cuts: each a sorted list of functions,
// the largest first.
func cycles(text string) ([][]string, error) {
	g, err := parse(text)
	if err != nil {
		return nil, err
	}
	nodes := map[string]bool{}
	for f := range g.adj {
		if !g.cuts[f] {
			nodes[f] = true
		}
	}
	found := [][]string{}
	for _, c := range g.components(nodes) {
		if len(c) > 1 || slices.Contains(g.adj[c[0]], c[0]) {
			slices.Sort(c)
			found = append(found, c)
		}
	}
	slices.SortFunc(found, func(a, b []string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return slices.Compare(a, b)
	})
	return found, nil
}

// path returns a shortest path of calls from src to dst, or nil.
func (g *graph) path(src, dst string) []string {
	if _, ok := g.adj[src]; !ok {
		return nil
	}
	parent := map[string]string{}
	seen := map[string]bool{src: true}
	work := []string{src}
	for len(work) > 0 {
		f := work[0]
		work = work[1:]
		if f == dst {
			var out []string
			for {
				out = append(out, f)
				if f == src {
					break
				}
				f = parent[f]
			}
			slices.Reverse(out)
			return out
		}
		for _, h := range g.adj[f] {
			if !seen[h] {
				seen[h] = true
				parent[h] = f
				work = append(work, h)
			}
		}
	}
	return nil
}

// reach is a pair where the first function reaches the second, and the path.
type reach struct {
	from, to string
	path     []string
}

// reaches returns the pairs where F reaches G, each with its path. A function
// of a pair that the module does not define, which would check nothing, is an
// error.
func reaches(text string, pairs []pair) ([]reach, error) {
	g, err := parse(text)
	if err != nil {
		return nil, err
	}
	var found []reach
	for _, p := range pairs {
		for _, h := range []string{p.from, p.to} {
			if _, ok := g.adj[h]; !ok {
				return nil, fmt.Errorf("%s of noReach is not in the module", h)
			}
		}
		if path := g.path(p.from, p.to); path != nil {
			found = append(found, reach{p.from, p.to, path})
		}
	}
	return found, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: cycles ENGINE.ll")
		os.Exit(2)
	}
	text, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cycles:", err)
		os.Exit(1)
	}
	cs, err := cycles(string(text))
	if err != nil {
		fmt.Fprintln(os.Stderr, "cycles:", err)
		os.Exit(1)
	}
	for _, c := range cs {
		fmt.Println(len(c), strings.Join(c, " "))
	}
	rs, err := reaches(string(text), noReach)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cycles:", err)
		os.Exit(1)
	}
	for _, r := range rs {
		fmt.Println("reach", r.from, r.to, "via", strings.Join(r.path, " "))
	}
}
