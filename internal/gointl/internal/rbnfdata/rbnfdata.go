// Package rbnfdata is the model layer for ICU's algorithmic numbering systems:
// the ones written by rules rather than by ten digits -- Roman and Hebrew
// numerals, the Japanese era year that calls its first year 元.
//
// What is stored is ICU's rule text, as ICU writes it, grouped as ICU groups
// it, and which rule set each numbering system is written by. The rules are
// the input; the numeral a number becomes is an answer, and not stored.
package rbnfdata

import (
	"fmt"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// A Group is one locale's group of rule sets, "ja/SpelloutRules": a rule set
// may call on any other in its group.
type Group struct {
	Name  string
	Rules []string
}

// A System is a numbering system and the rule set that writes it.
type System struct {
	Name, Group, Set string
}

// Data is every algorithmic numbering system and the rule groups they use.
type Data struct {
	Groups  []Group
	Systems []System
}

// Group finds a rule group by name.
func (d *Data) Group(name string) (*Group, bool) {
	for i := range d.Groups {
		if d.Groups[i].Name == name {
			return &d.Groups[i], true
		}
	}
	return nil, false
}

// System finds a numbering system by name.
func (d *Data) System(name string) (System, bool) {
	for _, s := range d.Systems {
		if s.Name == name {
			return s, true
		}
	}
	return System{}, false
}

// Encode writes the data.
func Encode(d *Data) []byte {
	w := blob.NewWriter(Version)
	w.Uint(len(d.Groups))
	for _, g := range d.Groups {
		w.String(g.Name)
		w.Uint(len(g.Rules))
		for _, r := range g.Rules {
			w.String(r)
		}
	}
	w.Uint(len(d.Systems))
	for _, s := range d.Systems {
		w.String(s.Name)
		w.String(s.Group)
		w.String(s.Set)
	}
	return w.Bytes()
}

// Decode reads what Encode wrote.
func Decode(b []byte) (*Data, error) {
	r, err := blob.NewReader(b, Version)
	if err != nil {
		return nil, err
	}
	count := func() (int, error) {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			return 0, fmt.Errorf("rbnfdata: a count of %d with %d bytes left", n, r.Left())
		}
		return n, nil
	}
	var d Data
	n, err := count()
	if err != nil {
		return nil, err
	}
	d.Groups = make([]Group, n)
	for i := range d.Groups {
		d.Groups[i].Name = r.String()
		m, err := count()
		if err != nil {
			return nil, err
		}
		d.Groups[i].Rules = make([]string, m)
		for j := range d.Groups[i].Rules {
			d.Groups[i].Rules[j] = r.String()
		}
	}
	if n, err = count(); err != nil {
		return nil, err
	}
	d.Systems = make([]System, n)
	for i := range d.Systems {
		d.Systems[i] = System{Name: r.String(), Group: r.String(), Set: r.String()}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &d, nil
}
