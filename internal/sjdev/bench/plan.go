// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// Plan is what a run is to time: every side at every position, in the rounds
// listed (in pairs 2k, 2k+1), the formal position FormalReps times a round.
// The judge reads the records against it, never against what the records
// happen to hold.
type Plan struct {
	Kind       string   `json:"kind"`
	Sides      []string `json:"sides"`
	Positions  []string `json:"positions"`
	Rounds     []int    `json:"rounds"`
	FormalReps int      `json:"formal_reps"`
	FreshPages bool     `json:"fresh_pages"`
}

// Reps is the number of blocks of position pos in a round.
func (p Plan) Reps(pos string) int {
	if pos == Formal.String() {
		return max(p.FormalReps, 1)
	}
	return 1
}

// Check reports what makes the plan unusable.
func (p Plan) Check() error {
	if len(p.Sides) < 2 || len(p.Positions) == 0 || len(p.Rounds) == 0 {
		return fmt.Errorf("the plan needs 2 sides or more, a position and a round: %+v", p)
	}
	for _, r := range p.Rounds {
		if r%2 == 0 && !slices.Contains(p.Rounds, r+1) || r%2 == 1 && !slices.Contains(p.Rounds, r-1) {
			return fmt.Errorf("round %d of the plan has no partner: rounds come in pairs 2k, 2k+1", r)
		}
	}
	return nil
}

// Manifest is the file next to the records (Out + ".manifest.json") that
// names the experiment: its plan, the hash of every binary and the settings
// that change what a process measures. A run that goes on in the same file
// must match it.
type Manifest struct {
	Experiment string            `json:"experiment"` // made at random when the file is started
	Plan       Plan              `json:"plan"`
	Binaries   map[string]string `json:"binaries"` // side@pos: SHA-256 of the file
	Parse      string            `json:"parse"`
	Args       []string          `json:"args"`
	Dir        string            `json:"dir"`
	Env        []string          `json:"env"`
	CPU        int               `json:"cpu"`
	Sib        int               `json:"sib"`
	BusyPct    float64           `json:"busy_pct"`
	StealPct   float64           `json:"steal_pct"`
}

// ManifestPath is the manifest of a file of records.
func ManifestPath(out string) string { return out + ".manifest.json" }

// ReadManifest reads the manifest of a file of records. ok is false when it
// does not exist.
func ReadManifest(out string) (m Manifest, ok bool, err error) {
	b, err := os.ReadFile(ManifestPath(out))
	if os.IsNotExist(err) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, false, fmt.Errorf("%s: %w", ManifestPath(out), err)
	}
	return m, true, nil
}

// WriteManifest writes the manifest of a file of records.
func WriteManifest(out string, m Manifest) error {
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(ManifestPath(out), append(b, '\n'), 0o644)
}

// HashFile returns the SHA-256 of a file, in hex.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// NewExperiment returns a new random experiment id.
func NewExperiment() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Same reports how m differs from the manifest o of the same file, in all but
// the experiment id and the rounds, which a run may add to.
func (m Manifest) Same(o Manifest) error {
	var diffs []string
	eq := func(name string, a, b any) {
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		if string(x) != string(y) {
			diffs = append(diffs, fmt.Sprintf("%s: %s in the file, %s now", name, x, y))
		}
	}
	eq("kind", o.Plan.Kind, m.Plan.Kind)
	eq("sides", o.Plan.Sides, m.Plan.Sides)
	eq("positions", o.Plan.Positions, m.Plan.Positions)
	eq("formal reps", o.Plan.FormalReps, m.Plan.FormalReps)
	eq("fresh pages", o.Plan.FreshPages, m.Plan.FreshPages)
	eq("binaries", o.Binaries, m.Binaries)
	eq("parse", o.Parse, m.Parse)
	eq("args", o.Args, m.Args)
	eq("dir", o.Dir, m.Dir)
	eq("env", o.Env, m.Env)
	eq("cpu", o.CPU, m.CPU)
	eq("sib", o.Sib, m.Sib)
	eq("busy", o.BusyPct, m.BusyPct)
	eq("steal", o.StealPct, m.StealPct)
	if len(diffs) > 0 {
		return fmt.Errorf("the run is not the experiment of the file:\n  %s", strings.Join(diffs, "\n  "))
	}
	return nil
}

// Begin makes the manifest of a run, or checks the run against the manifest
// the file already has and adds its rounds to it, then writes it. A file with
// records of the kind but no manifest is refused: nothing says what they are.
func Begin(o RunOptions, vs []Variant, old []Record) (Manifest, error) {
	m := Manifest{
		Plan:     Plan{Kind: o.Kind, FormalReps: o.FormalReps, FreshPages: o.FreshPages},
		Binaries: map[string]string{},
		Parse:    o.Parse, Args: o.Args, Dir: o.Dir, Env: o.Env,
		CPU: o.CPU, Sib: o.Sib, BusyPct: o.BusyPct, StealPct: o.StealPct,
	}
	for _, v := range vs {
		if !slices.Contains(m.Plan.Sides, v.Side) {
			m.Plan.Sides = append(m.Plan.Sides, v.Side)
		}
		if !slices.Contains(m.Plan.Positions, v.Pos) {
			m.Plan.Positions = append(m.Plan.Positions, v.Pos)
		}
		h, err := HashFile(v.Path)
		if err != nil {
			return m, err
		}
		m.Binaries[v.Side+"@"+v.Pos] = h
	}
	slices.Sort(m.Plan.Sides)
	slices.Sort(m.Plan.Positions)
	for rd := o.Round0; rd < o.Round0+o.Rounds; rd++ {
		m.Plan.Rounds = append(m.Plan.Rounds, rd)
	}
	prev, ok, err := ReadManifest(o.Out)
	if err != nil {
		return m, err
	}
	if !ok {
		for _, r := range old {
			if r.Kind == o.Kind {
				return m, fmt.Errorf("%s has records of %s but no manifest: write to another file", o.Out, o.Kind)
			}
		}
		m.Experiment = NewExperiment()
	} else {
		if err := m.Same(prev); err != nil {
			return m, fmt.Errorf("%s: %w", o.Out, err)
		}
		m.Experiment = prev.Experiment
		rounds := slices.Clone(prev.Plan.Rounds)
		for _, rd := range m.Plan.Rounds {
			if !slices.Contains(rounds, rd) {
				rounds = append(rounds, rd)
			}
		}
		slices.Sort(rounds)
		m.Plan.Rounds = rounds
	}
	if err := m.Plan.Check(); err != nil {
		return m, err
	}
	return m, WriteManifest(o.Out, m)
}
