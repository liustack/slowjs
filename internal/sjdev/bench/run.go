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

// Record is one process, written as a line of JSON. A block is the processes
// of every side at one position in one round (and rep): they run one after
// the other, in the order Order, and run again together when one of them was
// disturbed, with the next Attempt.
type Record struct {
	Side          string             `json:"side"`
	Pos           string             `json:"pos"`
	Kind          string             `json:"kind"`
	Round         int                `json:"round"`
	Rep           int                `json:"rep,omitempty"`
	Attempt       int                `json:"attempt"`
	Slot          int                `json:"slot"`            // place of the process in its block
	Order         string             `json:"order,omitempty"` // the sides of the block, in the order they ran
	Session       string             `json:"session,omitempty"`
	Experiment    string             `json:"experiment,omitempty"`     // the id of the manifest of the file
	Start         string             `json:"start,omitempty"`          // RFC 3339, UTC
	FreshPages    bool               `json:"fresh_pages,omitempty"`    // dropping the cached pages of the binary was asked before it ran
	FreshVerified bool               `json:"fresh_verified,omitempty"` // and mincore found none of them left in memory
	Metrics       map[string]float64 `json:"metrics,omitempty"`
	HWMKB         int64              `json:"vmhwm_kb,omitempty"`
	WallMs        float64            `json:"wall_ms"`
	CPUMs         float64            `json:"cpu_ms"`
	SibPct        float64            `json:"sib_pct"`     // the sibling core busy, in percent of the wall time
	OtherPct      float64            `json:"other_pct"`   // other work on the timing core
	StealPct      float64            `json:"steal_pct"`   // steal time on the timing core
	MachinePct    float64            `json:"machine_pct"` // all other cores busy, summed (100 is one core)
	Disturbed     bool               `json:"disturbed,omitempty"`
	Err           string             `json:"err,omitempty"`
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

// CPUTicks are the ticks of one cpu: busy is everything but idle and iowait,
// steal is the time the host gave to other machines.
type CPUTicks struct{ Busy, Steal int64 }

// Ticks returns the ticks of each cpu, or nil where the machine does not say.
type Ticks func() map[int]CPUTicks

// ProcTicks reads /proc/stat.
func ProcTicks() map[int]CPUTicks {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	return ParseTicks(string(b))
}

// ParseTicks parses the text of /proc/stat.
func ParseTicks(stat string) map[int]CPUTicks {
	m := map[int]CPUTicks{}
	for _, l := range strings.Split(stat, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		id, err := strconv.Atoi(f[0][3:])
		if err != nil {
			continue
		}
		var sum, idle, steal int64
		for i, v := range f[1:] {
			n, _ := strconv.ParseInt(v, 10, 64)
			// guest and guest_nice are already in user and nice.
			if i >= 8 {
				break
			}
			sum += n
			if i == 3 || i == 4 {
				idle += n
			}
			if i == 7 {
				steal = n
			}
		}
		m[id] = CPUTicks{Busy: sum - idle - steal, Steal: steal}
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
	Bin        string   // folder of the binaries (side@pos)
	Out        string   // the file of records, appended to
	Kind       string   // label of this kind of measurement in the file
	Rounds     int      // number of rounds, even, so that every two sides meet in both orders
	Round0     int      // number of the first round, even
	FormalReps int      // blocks of the formal layout in each round (the others have one)
	Sides      []string // limit to these sides (empty: all)
	Pos        []string // limit to these positions (empty: all)
	Parse      string   // see ParserFor
	Args       []string // arguments of the process
	Dir        string   // working folder of the process
	Env        []string // K=V
	CPU        int      // the timing core (-1: do not check)
	Sib        int      // its hyperthread sibling, which must stay idle (-1: none)
	BusyPct    float64  // percent of a busy sibling or other work on the timing core that disturbs a block
	StealPct   float64  // percent of steal time on the timing core that disturbs a block
	Tries      int      // tries of a disturbed block
	Timeout    time.Duration
	MinTicks   int64 // fewer busy ticks than this are noise
	Session    string
	FreshPages bool   // drop the cached pages of a binary before each of its processes (DropPages)
	Experiment string // set by Run from the manifest
}

// Defaults fills what is not set.
func (o *RunOptions) Defaults() {
	if o.Kind == "" {
		o.Kind = "run"
	}
	if o.FormalReps == 0 {
		o.FormalReps = 1
	}
	if o.BusyPct == 0 {
		o.BusyPct = 5
	}
	if o.StealPct == 0 {
		o.StealPct = 1
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
	if o.Session == "" {
		host, _ := os.Hostname()
		o.Session = fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
	}
}

// Variant is one binary.
type Variant struct{ Side, Pos, Path string }

// Discover lists the binaries of dir, limited by sides and positions. Every
// side asked for (or every side found) must have a binary at every position
// asked for (or found).
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
	// The plan is every side asked for (or found) at every position asked for
	// (or found), and each of those binaries must be there: one that is missing
	// is an error, not a smaller plan.
	have := map[string]bool{}
	var allSides, allPos []string
	for _, v := range vs {
		have[v.Side+"@"+v.Pos] = true
		if !slices.Contains(allSides, v.Side) {
			allSides = append(allSides, v.Side)
		}
		if !slices.Contains(allPos, v.Pos) {
			allPos = append(allPos, v.Pos)
		}
	}
	if len(sides) > 0 {
		allSides = sides
	}
	if len(pos) > 0 {
		allPos = pos
	}
	for _, s := range allSides {
		for _, p := range allPos {
			if !have[s+"@"+p] {
				return nil, fmt.Errorf("side %s has no binary at position %s in %s", s, p, dir)
			}
		}
	}
	return vs, nil
}

func seeded(s string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(s))
	return rand.New(rand.NewPCG(h.Sum64(), 1))
}

// Block is the processes of one layout in a round: every side once, one after
// the other.
type Block struct {
	Pos   string
	Rep   int // which block of the formal layout in the round
	Sides []Variant
}

// Order returns the blocks of one round. The layouts come in a random order,
// the formal one reps times. In a block the sides come in a random order in an
// even round and in the reverse of that order in the odd round after it, so
// every two sides meet as often in one order as in the other. The order depends
// only on seed (the experiment of the file, so a new experiment gets new
// orders) and the round, so that a restart repeats it.
func Order(vs []Variant, seed string, round, reps int) []Block {
	var positions []string
	by := map[string][]Variant{}
	for _, v := range vs {
		if _, ok := by[v.Pos]; !ok {
			positions = append(positions, v.Pos)
		}
		by[v.Pos] = append(by[v.Pos], v)
	}
	slices.Sort(positions)
	var blocks []Block
	for _, p := range positions {
		n := 1
		if p == Formal.String() {
			n = max(reps, 1)
		}
		for rep := range n {
			blocks = append(blocks, Block{Pos: p, Rep: rep})
		}
	}
	seeded(fmt.Sprintf("%s-pos-%d", seed, round)).Shuffle(len(blocks), func(i, j int) { blocks[i], blocks[j] = blocks[j], blocks[i] })
	for i := range blocks {
		group := slices.Clone(by[blocks[i].Pos])
		slices.SortFunc(group, func(a, b Variant) int { return strings.Compare(a.Side, b.Side) })
		seeded(fmt.Sprintf("%s-side-%s-%d-%d", seed, blocks[i].Pos, blocks[i].Rep, round/2)).Shuffle(len(group), func(i, j int) { group[i], group[j] = group[j], group[i] })
		if round%2 == 1 {
			slices.Reverse(group)
		}
		blocks[i].Sides = group
	}
	return blocks
}

type blockKey struct {
	pos, kind  string
	round, rep int
}

// blockState is what the file already has of a block.
type blockState struct {
	attempts int  // the highest attempt seen, plus one
	done     bool // the last attempt has every side and needs no rerun
}

// Run times the binaries block after block, round after round, and appends a
// record per process to o.Out. A block where any process was disturbed runs
// again whole, up to o.Tries times. Blocks already in the file are not run
// again, so a stopped run goes on where it stopped.
func Run(ctx context.Context, o RunOptions, start Starter, ticks Ticks, w io.Writer) error {
	o.Defaults()
	if o.Rounds%2 != 0 || o.Round0%2 != 0 {
		return fmt.Errorf("-rounds %d and -round0 %d must be even, so that the sides meet in both orders", o.Rounds, o.Round0)
	}
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
	m, err := Begin(o, vs, old)
	if err != nil {
		return err
	}
	o.Experiment = m.Experiment
	nsides := map[string]bool{}
	for _, v := range vs {
		nsides[v.Side] = true
	}
	state := map[blockKey]*blockState{}
	type att struct {
		k blockKey
		a int
	}
	sidesOf := map[att]map[string]Record{}
	for _, r := range old {
		if r.Kind != o.Kind {
			continue
		}
		k := blockKey{r.Pos, r.Kind, r.Round, r.Rep}
		if state[k] == nil {
			state[k] = &blockState{}
		}
		state[k].attempts = max(state[k].attempts, r.Attempt+1)
		a := att{k, r.Attempt}
		if sidesOf[a] == nil {
			sidesOf[a] = map[string]Record{}
		}
		sidesOf[a][r.Side] = r
	}
	for k, s := range state {
		last := sidesOf[att{k, s.attempts - 1}]
		whole, disturbed := len(last) >= len(nsides), false
		for side := range nsides {
			r, ok := last[side]
			if !ok || r.Err != "" {
				whole = false
			}
			disturbed = disturbed || r.Disturbed
		}
		s.done = whole && (!disturbed || s.attempts >= o.Tries)
	}
	f, err := os.OpenFile(o.Out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for rd := o.Round0; rd < o.Round0+o.Rounds; rd++ {
		t0 := time.Now()
		reruns := 0
		for _, b := range Order(vs, o.Experiment, rd, o.FormalReps) {
			k := blockKey{b.Pos, o.Kind, rd, b.Rep}
			s := state[k]
			if s == nil {
				s = &blockState{}
				state[k] = s
			}
			for !s.done {
				disturbed := false
				order := make([]string, len(b.Sides))
				for i, v := range b.Sides {
					order[i] = v.Side
				}
				for slot, v := range b.Sides {
					r := once(ctx, o, v, rd, s.attempts, parse, start, ticks)
					r.Rep, r.Slot, r.Order = b.Rep, slot, strings.Join(order, ",")
					bs, _ := json.Marshal(r)
					if _, err := f.Write(append(bs, '\n')); err != nil {
						return err
					}
					if r.Err != "" {
						return fmt.Errorf("%s@%s round %d: %s", v.Side, v.Pos, rd, r.Err)
					}
					disturbed = disturbed || r.Disturbed
				}
				s.attempts++
				s.done = !disturbed || s.attempts >= o.Tries
				if disturbed {
					reruns++
				}
			}
		}
		fmt.Fprintf(w, "round %d done in %.0f s, %d blocks disturbed\n", rd, time.Since(t0).Seconds(), reruns)
	}
	return nil
}

func once(ctx context.Context, o RunOptions, v Variant, round, attempt int, parse Parser, start Starter, ticks Ticks) Record {
	r := Record{Side: v.Side, Pos: v.Pos, Kind: o.Kind, Round: round, Attempt: attempt, Session: o.Session, Experiment: o.Experiment, FreshPages: o.FreshPages}
	pctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if o.FreshPages {
		if err := DropPages(v.Path); err != nil {
			r.Err = "drop pages: " + err.Error()
			return r
		}
		r.FreshVerified = true
	}
	var t0 map[int]CPUTicks
	if ticks != nil {
		t0 = ticks()
	}
	begin := time.Now()
	r.Start = begin.UTC().Format(time.RFC3339Nano)
	p := start(pctx, v.Path, o.Args, o.Dir, o.Env)
	var t1 map[int]CPUTicks
	if ticks != nil {
		t1 = ticks()
	}
	r.WallMs = float64(p.Wall) / float64(time.Millisecond)
	r.CPUMs = float64(p.CPU) / float64(time.Millisecond)
	if t0 != nil && t1 != nil && p.Wall > 0 {
		wall := p.Wall.Seconds()
		d := func(c int) CPUTicks {
			return CPUTicks{t1[c].Busy - t0[c].Busy, t1[c].Steal - t0[c].Steal}
		}
		sib := d(o.Sib).Busy
		cpu := d(o.CPU)
		other := cpu.Busy - int64(p.CPU.Seconds()*100)
		var machine int64
		for c := range t1 {
			if c != o.CPU && c != o.Sib {
				machine += d(c).Busy
			}
		}
		r.SibPct, r.OtherPct = float64(sib)/wall, float64(max(other, 0))/wall
		r.StealPct, r.MachinePct = float64(cpu.Steal)/wall, float64(machine)/wall
		r.Disturbed = (o.Sib >= 0 && r.SibPct > o.BusyPct && sib >= o.MinTicks) ||
			(o.CPU >= 0 && r.OtherPct > o.BusyPct && other >= o.MinTicks) ||
			(o.CPU >= 0 && r.StealPct > o.StealPct && cpu.Steal >= 1)
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
