// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/liustack/quickjs/internal/sjdev"
)

// Profile is the self cost of each function in a callgrind file, for every
// event it counted (Ir, and with the simulations Bc, Bcm, I1mr and the others).
type Profile struct {
	Events []string
	Fn     map[string][]float64
	Total  []float64
}

// Event returns the column of an event, or -1.
func (p Profile) Event(name string) int {
	for i, e := range p.Events {
		if e == name {
			return i
		}
	}
	return -1
}

// ParseProfile reads a callgrind file written with --compress-strings=no and
// --compress-pos=no. A cost line right after calls= is the inclusive cost of
// the call and is not the caller's own.
func ParseProfile(r io.Reader) (Profile, error) {
	p := Profile{Fn: map[string][]float64{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	npos, cur, skip := 1, "", false
	var totals []float64
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "events:"):
			p.Events = strings.Fields(l[len("events:"):])
			continue
		case strings.HasPrefix(l, "positions:"):
			npos = len(strings.Fields(l[len("positions:"):]))
			continue
		case strings.HasPrefix(l, "fn="):
			cur = l[3:]
			if strings.HasPrefix(cur, "(") {
				return p, fmt.Errorf("compressed names: run callgrind with --compress-strings=no")
			}
			if i := strings.LastIndex(cur, "'"); i > 0 {
				cur = cur[:i] // the recursion depth of callgrind
			}
			continue
		case strings.HasPrefix(l, "calls="):
			skip = true
			continue
		case strings.HasPrefix(l, "totals:"):
			for _, f := range strings.Fields(l[len("totals:"):]) {
				x, err := strconv.ParseFloat(f, 64)
				if err != nil {
					return p, fmt.Errorf("totals: %w", err)
				}
				totals = append(totals, x)
			}
			continue
		}
		if l == "" || !(l[0] >= '0' && l[0] <= '9' || l[0] == '+' || l[0] == '-' || l[0] == '*') {
			continue
		}
		if skip {
			skip = false
			continue
		}
		fs := strings.Fields(l)
		if len(fs) < npos || cur == "" {
			continue
		}
		if fs[0] == "*" || strings.HasPrefix(fs[0], "+") || strings.HasPrefix(fs[0], "-") {
			return p, fmt.Errorf("compressed positions: run callgrind with --compress-pos=no")
		}
		v := p.Fn[cur]
		if v == nil {
			v = make([]float64, len(p.Events))
			p.Fn[cur] = v
		}
		for i, s := range fs[npos:] {
			if i >= len(v) {
				break
			}
			x, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return p, fmt.Errorf("cost line %q: %w", l, err)
			}
			v[i] += x
		}
	}
	if err := sc.Err(); err != nil {
		return p, err
	}
	if len(p.Events) == 0 {
		return p, fmt.Errorf("no events line: not a callgrind file")
	}
	p.Total = make([]float64, len(p.Events))
	for _, v := range p.Fn {
		for i := range v {
			p.Total[i] += v[i]
		}
	}
	// The sum of the self costs must be the total the file states.
	for i := range min(len(totals), len(p.Total)) {
		if math.Abs(totals[i]-p.Total[i]) > 0.5 {
			return p, fmt.Errorf("event %s: the functions add up to %.0f, the file says %.0f", p.Events[i], p.Total[i], totals[i])
		}
	}
	return p, nil
}

// Scale divides every count by n.
func (p Profile) Scale(n float64) Profile {
	q := Profile{Events: p.Events, Fn: map[string][]float64{}, Total: make([]float64, len(p.Total))}
	for i, x := range p.Total {
		q.Total[i] = x / n
	}
	for k, v := range p.Fn {
		w := make([]float64, len(v))
		for i, x := range v {
			w[i] = x / n
		}
		q.Fn[k] = w
	}
	return q
}

// fnRow finds the function whose name contains name, the largest by the first event.
func (p Profile) fnRow(name string) []float64 {
	var best []float64
	for k, v := range p.Fn {
		if strings.Contains(k, name) && (best == nil || v[0] > best[0]) {
			best = v
		}
	}
	return best
}

// WriteProfiles writes the counts per render of the whole program and of the
// interpreter's own code for each event, and, with a second profile, the change.
// The counts explain a timing; they do not decide.
func WriteProfiles(w io.Writer, interp string, a Profile, b *Profile) {
	rows := []struct {
		name string
		x, y []float64
	}{{"whole program", a.Total, nil}, {interp + " (own code)", a.fnRow(interp), nil}}
	if b != nil {
		rows[0].y, rows[1].y = b.Total, b.fnRow(interp)
	}
	fmt.Fprintf(w, "| part | event | per render | ")
	if b != nil {
		fmt.Fprintf(w, "second | change |\n| --- | --- | ---: | ---: | ---: |\n")
	} else {
		fmt.Fprintf(w, "\n| --- | --- | ---: |\n")
	}
	for _, r := range rows {
		if r.x == nil {
			continue
		}
		for i, e := range a.Events {
			if b == nil {
				fmt.Fprintf(w, "| %s | %s | %.0f |\n", r.name, e, r.x[i])
				continue
			}
			j := b.Event(e)
			if j < 0 || r.y == nil {
				continue
			}
			ch := "-"
			if r.x[i] != 0 {
				ch = fmt.Sprintf("%+.2f%%", 100*(r.y[j]/r.x[i]-1))
			}
			fmt.Fprintf(w, "| %s | %s | %.0f | %.0f | %s |\n", r.name, e, r.x[i], r.y[j], ch)
		}
	}
}

// WriteFnTable writes every function with its counts per render, largest Ir first, as TSV.
func WriteFnTable(w io.Writer, p Profile) {
	fmt.Fprintf(w, "fn\t%s\n", strings.Join(p.Events, "\t"))
	keys := make([]string, 0, len(p.Fn))
	for k := range p.Fn {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return p.Fn[keys[i]][0] > p.Fn[keys[j]][0] })
	row := func(k string, v []float64) {
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = strconv.FormatFloat(x, 'f', 1, 64)
		}
		fmt.Fprintf(w, "%s\t%s\n", k, strings.Join(s, "\t"))
	}
	row("TOTAL", p.Total)
	for _, k := range keys {
		row(k, p.Fn[k])
	}
}

// cgWindow runs each binary once under callgrind, counting only in the window:
// the dump the program writes around its o.Big renders after its own warm-up (or the inside of o.Toggle), keeps the raw file
// and a table per function, and prints the counts per render.
func cgWindow(ctx context.Context, o CGOptions, run sjdev.Runner, w io.Writer) error {
	vg, vgEnv := o.ValgrindCmd()
	env := append(append([]string(nil), o.Env...), vgEnv...)
	var ps []Profile
	for i, bin := range o.Bins {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return err
		}
		base := filepath.Join(o.Out, fmt.Sprintf("%d-%s-%d", i, filepath.Base(bin), o.Big))
		r := run(ctx, sjdev.Cmd{Dir: o.Dir, Env: env, Name: vg, Args: o.CGArgs(abs, base+".cg", o.Big)}, nil)
		if r.Err != nil {
			return fmt.Errorf("%w\n%s", r.Err, trunc(r.Out, 2000))
		}
		if err := os.WriteFile(base+".log", []byte(r.Out), 0o644); err != nil {
			return err
		}
		file := base + ".cg"
		if o.Dumps {
			ds, err := filepath.Glob(base + ".cg.*")
			if err != nil {
				return err
			}
			if len(ds) != 1 {
				return fmt.Errorf("%s: want one dump of the window, found %d (%v)", base, len(ds), ds)
			}
			file = ds[0]
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		p, err := ParseProfile(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if p.Event("Ir") != 0 || p.Total[0] == 0 {
			return fmt.Errorf("%s: nothing counted in the window", file)
		}
		p = p.Scale(float64(o.Big))
		t, err := os.Create(base + ".fn.tsv")
		if err != nil {
			return err
		}
		WriteFnTable(t, p)
		t.Close()
		ps = append(ps, p)
	}
	fmt.Fprintf(w, "counts per render (each page once), in the window\n\n")
	if len(ps) == 2 {
		WriteProfiles(w, o.Interp, ps[0], &ps[1])
	} else {
		WriteProfiles(w, o.Interp, ps[0], nil)
	}
	return nil
}
