// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/liustack/slowjs/internal/sjdev"
)

var cgLine = regexp.MustCompile(`^\s*([\d,]+) \(\s*[\d.]+%\)\s+(.*)$`)

// Listing is what callgrind_annotate --inclusive=yes printed for one run: the
// total and the largest inclusive count of each function name.
type Listing struct {
	Total float64
	Fn    map[string]float64
}

// ParseListing reads the text of callgrind_annotate.
func ParseListing(text string) Listing {
	l := Listing{Fn: map[string]float64{}}
	for _, line := range strings.Split(text, "\n") {
		m := cgLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		name := m[2]
		if strings.HasPrefix(name, "PROGRAM TOTALS") {
			l.Total = n
			continue
		}
		if i := strings.LastIndex(name, " ["); i > 0 {
			name = name[:i]
		}
		if i := strings.Index(name, ".go:"); i > 0 {
			name = name[i+4:]
		} else if i := strings.Index(name, ".s:"); i > 0 {
			name = name[i+3:]
		} else if i := strings.LastIndex(name, ":"); i > 0 {
			name = name[i+1:]
		}
		if strings.Contains(name, "'") || strings.HasSuffix(name, "x)") {
			continue // a recursive entry counts its instructions again
		}
		l.Fn[name] = max(l.Fn[name], n)
	}
	return l
}

// PerRender is the instructions of one render: what the big run did beyond the
// small run, divided by the renders it did more. What loading and compiling
// cost is in both runs, so it cancels.
type PerRender struct {
	Total float64
	Fn    map[string]float64
}

// Diff computes the instructions per render from two listings.
func Diff(renders float64, small, big Listing) PerRender {
	d := PerRender{Total: (big.Total - small.Total) / renders, Fn: map[string]float64{}}
	for k, v := range big.Fn {
		d.Fn[k] = (v - small.Fn[k]) / renders
	}
	return d
}

func shortName(s string) string {
	for _, p := range []string{"github.com/liustack/slowjs/internal/", "github.com/liustack/slowjs.", "github.com/liustack/tifa/internal/"} {
		s = strings.TrimPrefix(s, p)
	}
	return s
}

// WriteDiff prints the functions that take at least 1/2000 of the total, largest
// first. With a second run it prints both and the change.
func WriteDiff(w io.Writer, a PerRender, b *PerRender) {
	if b == nil {
		fmt.Fprintf(w, "%-60s %14.0f 100.00%%\n", "per render", a.Total)
		keys := make([]string, 0, len(a.Fn))
		for k := range a.Fn {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return a.Fn[keys[i]] > a.Fn[keys[j]] })
		for _, k := range keys {
			if a.Fn[k] < a.Total/2000 {
				break
			}
			fmt.Fprintf(w, "%-60s %14.0f %6.2f%%\n", shortName(k), a.Fn[k], 100*a.Fn[k]/a.Total)
		}
		return
	}
	fmt.Fprintf(w, "%-60s %14.0f %14.0f %+7.2f%%\n", "per render", a.Total, b.Total, 100*(b.Total-a.Total)/a.Total)
	seen := map[string]bool{}
	var list []string
	for k := range a.Fn {
		seen[k] = true
		list = append(list, k)
	}
	for k := range b.Fn {
		if !seen[k] {
			list = append(list, k)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return max(a.Fn[list[i]], b.Fn[list[i]]) > max(a.Fn[list[j]], b.Fn[list[j]])
	})
	for _, k := range list {
		if max(a.Fn[k], b.Fn[k]) < a.Total/2000 {
			break
		}
		fmt.Fprintf(w, "%-60s %14.0f %14.0f %6.2f%% %6.2f%%\n", shortName(k), a.Fn[k], b.Fn[k], 100*a.Fn[k]/a.Total, 100*b.Fn[k]/b.Total)
	}
}

// CGOptions say what to count.
type CGOptions struct {
	Bins         []string // one or two binaries
	Small, Big   int      // renders of the small and of the big run
	Args         []string // arguments, with {N} replaced by the renders
	Dir          string
	Env          []string
	Toggle       string // collect only inside this function (empty: the whole run)
	ValgrindRoot string // a folder with usr/bin/valgrind inside, when valgrind is not installed
	Out          string // folder of the callgrind files
}

// Check reports why the options cannot be used.
func (o CGOptions) Check() error {
	if len(o.Bins) < 1 || len(o.Bins) > 2 {
		return fmt.Errorf("give one binary, or two to compare")
	}
	if o.Small < 0 || o.Big <= o.Small {
		return fmt.Errorf("-small %d and -big %d: the big run must do more renders", o.Small, o.Big)
	}
	if o.Out == "" {
		return fmt.Errorf("no output folder: pass -out")
	}
	if !slicesContainsN(o.Args) {
		return fmt.Errorf("the arguments have no {N}: put the number of renders there, such as -test.benchtime={N}x")
	}
	return nil
}

func slicesContainsN(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "{N}") {
			return true
		}
	}
	return false
}

// ValgrindCmd returns the program and the environment of valgrind.
func (o CGOptions) ValgrindCmd() (string, []string) {
	if o.ValgrindRoot == "" {
		return "valgrind", nil
	}
	return filepath.Join(o.ValgrindRoot, "usr", "bin", "valgrind"), []string{"VALGRIND_LIB=" + filepath.Join(o.ValgrindRoot, "usr", "libexec", "valgrind")}
}

func (o CGOptions) annotate() string {
	if o.ValgrindRoot == "" {
		return "callgrind_annotate"
	}
	return filepath.Join(o.ValgrindRoot, "usr", "bin", "callgrind_annotate")
}

// CGArgs returns the arguments of valgrind for one run.
func (o CGOptions) CGArgs(bin, out string, n int) []string {
	args := []string{"--tool=callgrind", "--callgrind-out-file=" + out}
	if o.Toggle != "" {
		args = append(args, "--toggle-collect="+o.Toggle)
	}
	args = append(args, bin)
	for _, a := range o.Args {
		args = append(args, strings.ReplaceAll(a, "{N}", strconv.Itoa(n)))
	}
	return args
}

// CG runs callgrind on each binary with the small and the big count, and prints
// the instructions per render.
func CG(ctx context.Context, o CGOptions, run sjdev.Runner, w io.Writer) error {
	if err := o.Check(); err != nil {
		return err
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return err
	}
	vg, vgEnv := o.ValgrindCmd()
	env := append(append([]string(nil), o.Env...), vgEnv...)
	// callgrind_annotate is a perl script that finds its parts through the same variable.
	var results []PerRender
	for i, bin := range o.Bins {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return err
		}
		var ls [2]Listing
		for j, n := range []int{o.Small, o.Big} {
			base := filepath.Join(o.Out, fmt.Sprintf("%d-%s-%d", i, filepath.Base(bin), n))
			r := run(ctx, sjdev.Cmd{Dir: o.Dir, Env: env, Name: vg, Args: o.CGArgs(abs, base+".cg", n)}, nil)
			if r.Err != nil {
				return fmt.Errorf("%w\n%s", r.Err, trunc(r.Out, 2000))
			}
			a := run(ctx, sjdev.Cmd{Env: vgEnv, Name: o.annotate(), Args: []string{"--inclusive=yes", "--threshold=100", base + ".cg"}}, nil)
			if a.Err != nil {
				return fmt.Errorf("%w\n%s", a.Err, trunc(a.Out, 2000))
			}
			if err := os.WriteFile(base+".incl", []byte(a.Out), 0o644); err != nil {
				return err
			}
			ls[j] = ParseListing(a.Out)
			fmt.Fprintf(w, "%s with %d renders: %.0f instructions\n", filepath.Base(bin), n, ls[j].Total)
		}
		results = append(results, Diff(float64(o.Big-o.Small), ls[0], ls[1]))
	}
	fmt.Fprintln(w)
	if len(results) == 2 {
		WriteDiff(w, results[0], &results[1])
	} else {
		WriteDiff(w, results[0], nil)
	}
	return nil
}
