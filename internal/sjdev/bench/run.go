// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/liustack/slowjs/internal/sjdev/hwm"
)

// Record is one process, written as a line of JSON.
type Record struct {
	Side      string             `json:"side"`
	Pos       string             `json:"pos"`
	Kind      string             `json:"kind"`
	Round     int                `json:"round"`
	Attempt   int                `json:"attempt"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
	HWMKB     int64              `json:"vmhwm_kb,omitempty"`
	WallMs    float64            `json:"wall_ms"`
	CPUMs     float64            `json:"cpu_ms"`
	SibPct    float64            `json:"sib_pct"`
	OtherPct  float64            `json:"other_pct"`
	Disturbed bool               `json:"disturbed,omitempty"`
	Err       string             `json:"err,omitempty"`
}

// ReadRecords reads a file of records. A file that does not exist has none.
func ReadRecords(path string) ([]Record, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rs []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rs = append(rs, r)
	}
	return rs, sc.Err()
}

// Parser turns the output of a process into metrics.
type Parser func(out string) (map[string]float64, error)

var (
	benchLine = regexp.MustCompile(`(?m)^Benchmark(\S+)\s+\d+\s+(\S.*)$`)
	evalLine  = regexp.MustCompile(`(?m)^eval: \d+ runs, min \S+, median (\S+),`)
)

// ParserFor returns the parser for a -parse value: "gobench:UNIT" reads the
// UNIT metric of each Benchmark line of go test, "jsrun" reads the median of
// the eval line of jsrun.
func ParserFor(spec string) (Parser, error) {
	kind, arg, _ := strings.Cut(spec, ":")
	switch kind {
	case "gobench":
		if arg == "" {
			return nil, fmt.Errorf("parse %q: say the unit, such as gobench:p50-ms or gobench:ns/op", spec)
		}
		return func(out string) (map[string]float64, error) {
			m := map[string]float64{}
			for _, l := range benchLine.FindAllStringSubmatch(out, -1) {
				f := strings.Fields(l[2])
				for i := 0; i+1 < len(f); i += 2 {
					if f[i+1] != arg {
						continue
					}
					if v, err := strconv.ParseFloat(f[i], 64); err == nil {
						m[l[1]] = v
					}
				}
			}
			if len(m) == 0 {
				return nil, fmt.Errorf("no benchmark line with %s", arg)
			}
			return m, nil
		}, nil
	case "jsrun":
		return func(out string) (map[string]float64, error) {
			l := evalLine.FindStringSubmatch(out)
			if l == nil {
				return nil, fmt.Errorf("no eval line")
			}
			d, err := time.ParseDuration(l[1])
			if err != nil {
				return nil, err
			}
			return map[string]float64{"eval": float64(d) / float64(time.Millisecond)}, nil
		}, nil
	}
	return nil, fmt.Errorf("parse %q: want gobench:UNIT or jsrun", spec)
}

// Ticks returns the busy ticks (everything but idle and iowait) of each cpu,
// or nil where the machine does not say.
type Ticks func() map[int]int64

// ProcTicks reads /proc/stat.
func ProcTicks() map[int]int64 {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	return ParseTicks(string(b))
}

// ParseTicks parses the text of /proc/stat.
func ParseTicks(stat string) map[int]int64 {
	m := map[int]int64{}
	for _, l := range strings.Split(stat, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		id, err := strconv.Atoi(f[0][3:])
		if err != nil {
			continue
		}
		var sum, idle int64
		for i, v := range f[1:] {
			n, _ := strconv.ParseInt(v, 10, 64)
			sum += n
			if i == 3 || i == 4 {
				idle += n
			}
		}
		m[id] = sum - idle
	}
	return m
}

// Proc is the result of starting one process.
type Proc struct {
	Out  string
	CPU  time.Duration // user and system time of the process
	Wall time.Duration
	Err  error
}

// Starter starts a process and waits for it.
type Starter func(ctx context.Context, bin string, args []string, dir string, env []string) Proc

// Start is the Starter that runs the process.
func Start(ctx context.Context, bin string, args []string, dir string, env []string) Proc {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	t0 := time.Now()
	out, err := cmd.CombinedOutput()
	p := Proc{Out: string(out), Wall: time.Since(t0), Err: err}
	if ps := cmd.ProcessState; ps != nil {
		p.CPU = ps.UserTime() + ps.SystemTime()
	}
	return p
}

// RunOptions say what to time.
type RunOptions struct {
	Bin      string   // folder of the binaries (side@pos)
	Out      string   // the file of records, appended to
	Kind     string   // label of this kind of measurement in the file
	Rounds   int      // number of rounds
	Round0   int      // number of the first round
	Sides    []string // limit to these sides (empty: all)
	Pos      []string // limit to these positions (empty: all)
	Parse    string   // see ParserFor
	Args     []string // arguments of the process
	Dir      string   // working folder of the process
	Env      []string // K=V
	CPU      int      // the timing core (-1: do not check)
	Sib      int      // its hyperthread sibling, which must stay idle (-1: none)
	BusyPct  float64  // percent of a busy sibling or other work that makes a process run again
	Tries    int
	Timeout  time.Duration
	MinTicks int64 // fewer busy ticks than this are noise
}

// Defaults fills what is not set.
func (o *RunOptions) Defaults() {
	if o.Kind == "" {
		o.Kind = "run"
	}
	if o.BusyPct == 0 {
		o.BusyPct = 10
	}
	if o.Tries == 0 {
		o.Tries = 3
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Minute
	}
	if o.MinTicks == 0 {
		o.MinTicks = 3
	}
}

// Variant is one binary.
type Variant struct{ Side, Pos, Path string }

// Discover lists the binaries of dir, limited by sides and positions.
func Discover(dir string, sides, pos []string) ([]Variant, error) {
	// The processes run in another folder, so the paths must not depend on this one.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	names, err := Sorted(dir)
	if err != nil {
		return nil, err
	}
	var vs []Variant
	for _, n := range names {
		side, p, _ := strings.Cut(n, "@")
		if len(sides) > 0 && !slices.Contains(sides, side) {
			continue
		}
		if len(pos) > 0 && !slices.Contains(pos, p) {
			continue
		}
		vs = append(vs, Variant{side, p, filepath.Join(dir, n)})
	}
	if len(vs) == 0 {
		return nil, fmt.Errorf("no binaries named side@pos in %s", dir)
	}
	return vs, nil
}

func seeded(s string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(s))
	return rand.New(rand.NewPCG(h.Sum64(), 1))
}

// Order returns the processes of one round: the positions in a random order,
// and in each position the sides in a random order. The order depends only on
// the round and the kind, so that a restart repeats it.
func Order(vs []Variant, kind string, round int) []Variant {
	var positions []string
	by := map[string][]Variant{}
	for _, v := range vs {
		if _, ok := by[v.Pos]; !ok {
			positions = append(positions, v.Pos)
		}
		by[v.Pos] = append(by[v.Pos], v)
	}
	slices.Sort(positions)
	seeded(fmt.Sprintf("%s-pos-%d", kind, round)).Shuffle(len(positions), func(i, j int) { positions[i], positions[j] = positions[j], positions[i] })
	var out []Variant
	for _, p := range positions {
		group := by[p]
		seeded(fmt.Sprintf("%s-side-%s-%d", kind, p, round)).Shuffle(len(group), func(i, j int) { group[i], group[j] = group[j], group[i] })
		out = append(out, group...)
	}
	return out
}

type key struct {
	side, pos, kind string
	round           int
}

// Run times the binaries in turn, round after round, and appends a record per
// process to o.Out. Processes already in the file are not run again, so a
// stopped run goes on where it stopped.
func Run(ctx context.Context, o RunOptions, start Starter, ticks Ticks, w io.Writer) error {
	o.Defaults()
	parse, err := ParserFor(o.Parse)
	if err != nil {
		return err
	}
	vs, err := Discover(o.Bin, o.Sides, o.Pos)
	if err != nil {
		return err
	}
	old, err := ReadRecords(o.Out)
	if err != nil {
		return err
	}
	done, tried := map[key]bool{}, map[key]int{}
	for _, r := range old {
		k := key{r.Side, r.Pos, r.Kind, r.Round}
		tried[k]++
		if r.Err == "" && (!r.Disturbed || tried[k] >= o.Tries) {
			done[k] = true
		}
	}
	f, err := os.OpenFile(o.Out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for rd := o.Round0; rd < o.Round0+o.Rounds; rd++ {
		t0 := time.Now()
		for _, v := range Order(vs, o.Kind, rd) {
			k := key{v.Side, v.Pos, o.Kind, rd}
			for !done[k] {
				r := once(ctx, o, v, rd, tried[k], parse, start, ticks)
				tried[k]++
				b, _ := json.Marshal(r)
				if _, err := f.Write(append(b, '\n')); err != nil {
					return err
				}
				if r.Err != "" {
					return fmt.Errorf("%s@%s round %d: %s", v.Side, v.Pos, rd, r.Err)
				}
				done[k] = !r.Disturbed || tried[k] >= o.Tries
			}
		}
		fmt.Fprintf(w, "round %d done in %.0f s\n", rd, time.Since(t0).Seconds())
	}
	return nil
}

func once(ctx context.Context, o RunOptions, v Variant, round, attempt int, parse Parser, start Starter, ticks Ticks) Record {
	r := Record{Side: v.Side, Pos: v.Pos, Kind: o.Kind, Round: round, Attempt: attempt}
	pctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	var t0 map[int]int64
	if ticks != nil {
		t0 = ticks()
	}
	p := start(pctx, v.Path, o.Args, o.Dir, o.Env)
	var t1 map[int]int64
	if ticks != nil {
		t1 = ticks()
	}
	r.WallMs = float64(p.Wall) / float64(time.Millisecond)
	r.CPUMs = float64(p.CPU) / float64(time.Millisecond)
	if t0 != nil && t1 != nil && p.Wall > 0 {
		wall := p.Wall.Seconds()
		sib := t1[o.Sib] - t0[o.Sib]
		other := t1[o.CPU] - t0[o.CPU] - int64(p.CPU.Seconds()*100)
		r.SibPct, r.OtherPct = float64(sib)/wall, float64(max(other, 0))/wall
		r.Disturbed = (o.Sib >= 0 && r.SibPct > o.BusyPct && sib >= o.MinTicks) || (o.CPU >= 0 && r.OtherPct > o.BusyPct && other >= o.MinTicks)
	}
	if pctx.Err() != nil {
		r.Err = "timeout"
		return r
	}
	if p.Err != nil {
		r.Err = trunc(fmt.Sprintf("%v: %s", p.Err, p.Out), 2000)
		return r
	}
	m, err := parse(p.Out)
	if err != nil {
		r.Err = trunc(fmt.Sprintf("%v: %s", err, p.Out), 2000)
		return r
	}
	r.Metrics = m
	if kb, ok := hwm.Find(p.Out); ok {
		r.HWMKB = kb
	}
	return r
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
