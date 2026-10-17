// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"maps"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

// The tz data comes in two forms that differ in what they call daylight
// saving time. In the main form, which Linux distributions and Go's
// time/tzdata ship, a few zones have it behind their standard time:
// Europe/Dublin keeps IST in summer as its standard time and GMT in winter as
// daylight saving time. In the rearguard form, which ICU builds from and
// macOS ships, daylight saving time is always ahead of standard time. Go's
// IsDST reports what the data it reads says, so on Linux the daylight and
// standard names of those zones would come out swapped.
//
// The generator builds both forms of one tzdb release, finds the zones and
// links whose daylight saving flag differs between them at some time before
// 2038, and writes their periods in the rearguard form to -dst. The runtime
// takes the flag from there for those zones (see isDST).

// tzdbVersion is the tzdb release the table comes from: the one in the ICU
// data of the Chromium the golden test records with (zoneinfo64.res in
// icudtl.dat of Chromium 156 says 2026d). Go 1.27 embeds 2026c in
// time/tzdata.
const tzdbVersion = "2026d"

// tzdbSHA256 is the SHA-256 of tzdata<tzdbVersion>.tar.gz, as signed by the
// tz maintainer.
const tzdbSHA256 = "0cb2aa8e333c3dc049badc42a0c61f21987b8cd44e107fa900bad764aacc7767"

// dstLimit is where the comparison of the two forms and the table end. Past
// it the runtime takes the flag of the zone's last period with the same
// offset.
var dstLimit = time.Date(2038, 1, 1, 0, 0, 0, 0, time.UTC)

// testZones are the zones whose two forms go to -tzdata for the tests: three
// of the ones with negative daylight saving time, and two where a standard
// time follows a daylight saving time of a smaller offset, which a rule
// comparing neighbors gets wrong.
var testZones = []string{"Africa/Casablanca", "Africa/Windhoek", "America/Nome", "Europe/Dublin", "Europe/Minsk"}

func tzdbFile() string {
	if *tzdbPath != "" {
		return *tzdbPath
	}

	d, err := os.UserCacheDir()
	if err != nil {
		fail(err)
	}

	return filepath.Join(d, "quickjs-intl", "tzdata"+tzdbVersion+".tar.gz")
}

func fetchTZDB() {
	dst := tzdbFile()
	if _, err := os.Stat(dst); err == nil {
		return
	}

	url := "https://data.iana.org/time-zones/releases/tzdata" + tzdbVersion + ".tar.gz"
	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		fail(err)
	}

	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		fail(err)
	}

	if resp.StatusCode != http.StatusOK {
		fail(fmt.Errorf("%s: %s", url, resp.Status))
	}

	if err := checkTZDB(b); err != nil {
		fail(fmt.Errorf("%s: %v", url, err))
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		fail(err)
	}

	if err := os.WriteFile(dst, b, 0o644); err != nil {
		fail(err)
	}
}

func checkTZDB(b []byte) error {
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != tzdbSHA256 {
		return fmt.Errorf("SHA-256 %s, want %s", got, tzdbSHA256)
	}

	return nil
}

// tzdb is one release built in both forms.
type tzdb struct {
	main, rearguard string            // the zic output directories
	links           map[string]string // link name -> zone
	names           []string          // the zones and links, sorted
}

// buildTZDB unpacks the release into a temporary directory, makes main.zi
// and rearguard.zi with its Makefile and compiles both with zic. The caller
// removes dir.
func buildTZDB() (db *tzdb, dir string) {
	b, err := os.ReadFile(tzdbFile())
	if err != nil {
		fail(fmt.Errorf("%v (run with -fetch to download it)", err))
	}

	if err := checkTZDB(b); err != nil {
		fail(fmt.Errorf("%s: %v", tzdbFile(), err))
	}

	if dir, err = os.MkdirTemp("", "quickjs-tzdb-"); err != nil {
		fail(err)
	}

	src := filepath.Join(dir, "src")
	if err := untar(b, src); err != nil {
		fail(fmt.Errorf("%s: %v", tzdbFile(), err))
	}

	if v, err := os.ReadFile(filepath.Join(src, "version")); err != nil || strings.TrimSpace(string(v)) != tzdbVersion {
		fail(fmt.Errorf("%s: not tzdb %s (version file %q, %v)", tzdbFile(), tzdbVersion, v, err))
	}

	run(exec.Command("make", "-C", src, "main.zi", "rearguard.zi"))
	db = &tzdb{main: filepath.Join(dir, "main"), rearguard: filepath.Join(dir, "rearguard"), links: map[string]string{}}
	// Fat files list every transition up to 2037. Past a zone's last listed
	// transition Go works the periods out from the zone's rule, and there
	// ZoneBounds ends a leap year's last period a day early, before the time
	// asked about, so walking a slim file with it goes wrong.
	run(exec.Command(*zic, "-b", "fat", "-d", db.main, filepath.Join(src, "main.zi")))
	run(exec.Command(*zic, "-b", "fat", "-d", db.rearguard, filepath.Join(src, "rearguard.zi")))
	zi, err := os.ReadFile(filepath.Join(src, "main.zi"))
	if err != nil {
		fail(err)
	}

	for line := range strings.Lines(string(zi)) {
		if f := strings.Fields(line); len(f) >= 3 && f[0] == "Link" {
			db.links[f[2]] = f[1]
		}
	}
	err = filepath.WalkDir(db.main, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(db.main, p)
		db.names = append(db.names, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		fail(err)
	}

	slices.Sort(db.names)
	return db, dir
}

// untar writes the regular files of the gzipped tar b, which are all at its
// top level, to dir.
func untar(b []byte, dir string) error {
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	r := tar.NewReader(z)
	for {
		h, err := r.Next()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		if h.Typeflag != tar.TypeReg {
			continue
		}

		if strings.ContainsAny(h.Name, `/\`) || h.Name == "." || h.Name == ".." {
			return fmt.Errorf("unexpected file %q", h.Name)
		}

		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(dir, h.Name), data, 0o644); err != nil {
			return err
		}
	}
}

func run(cmd *exec.Cmd) {
	if out, err := cmd.CombinedOutput(); err != nil {
		fail(fmt.Errorf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, out))
	}
}

func (db *tzdb) load(form, name string) *time.Location {
	b, err := os.ReadFile(filepath.Join(form, filepath.FromSlash(name)))
	if err != nil {
		fail(err)
	}

	loc, err := time.LoadLocationFromTZData(name, b)
	if err != nil {
		fail(fmt.Errorf("%s: %v", name, err))
	}

	return loc
}

// dstPeriod is a time from start on (Unix seconds) with one offset and one
// daylight saving flag.
type dstPeriod struct {
	start  int64
	offset int
	dst    bool
}

// periods returns the periods of loc that start before dstLimit, the first
// one from the beginning of time. Next periods with the same offset and flag
// are one: zic splits a period where only the abbreviation or the rule
// changes.
func periods(loc *time.Location) (r []dstPeriod) {
	t := time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC).In(loc)
	start := int64(math.MinInt64)
	for {
		_, off := t.Zone()
		if n := len(r); n == 0 || r[n-1].offset != off || r[n-1].dst != t.IsDST() {
			r = append(r, dstPeriod{start, off, t.IsDST()})
		}
		_, end := t.ZoneBounds()
		if end.IsZero() || !end.Before(dstLimit) {
			return r
		}

		if !end.After(t) {
			fail(fmt.Errorf("%s: the period at %v ends at %v", loc, t, end))
		}

		t, start = end, end.Unix()
	}
}

// differs reports whether the daylight saving flag of the two forms of a
// zone differs at some time before dstLimit. Their offsets have to be the
// same: the runtime keeps the offset of the data it reads.
func differs(name string, main, rearguard []dstPeriod) bool {
	at := func(ps []dstPeriod, t int64) dstPeriod {
		i, _ := slices.BinarySearchFunc(ps, t, func(p dstPeriod, t int64) int {
			return cmp.Compare(p.start, t)
		})
		if i == len(ps) || ps[i].start != t {
			i--
		}
		return ps[i]
	}
	var starts []int64
	for _, p := range append(slices.Clone(main), rearguard...) {
		starts = append(starts, p.start)
	}
	slices.Sort(starts)
	r := false
	for _, t := range slices.Compact(starts) {
		m, g := at(main, t), at(rearguard, t)
		if m.offset != g.offset {
			fail(fmt.Errorf("tzdb %s: %s has offset %d in the main form and %d in the rearguard form at %d", tzdbVersion, name, m.offset, g.offset, t))
		}

		if m.dst != g.dst {
			r = true
		}
	}
	return r
}

// dstTable returns the source of -dst for db, and the names it holds.
func dstTable(db *tzdb) ([]byte, []string) {
	data := map[string][]dstPeriod{} // by zone
	var names []string
	for _, name := range db.names {
		g := periods(db.load(db.rearguard, name))
		if !differs(name, periods(db.load(db.main, name)), g) {
			continue
		}

		zone := name
		if z, ok := db.links[name]; ok {
			zone = z
		}
		if d, ok := data[zone]; ok && !slices.Equal(d, g) {
			fail(fmt.Errorf("tzdb %s: link %s differs from its zone %s", tzdbVersion, name, zone))
		}

		data[zone] = g
		names = append(names, name)
	}
	varName := func(zone string) string {
		return "dst" + strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, zone)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by "go run ./intl/internal/gen"; DO NOT EDIT.

package intl

// tzdbVersion is the tzdb release rearguardDST comes from.
const tzdbVersion = %q

// rearguardDST holds the zones and links whose daylight saving flag differs
// between the main and the rearguard form of the tz data before 2038: their
// periods in the rearguard form, each from its start (Unix seconds) to the
// next one's, the first from the beginning of time.
var rearguardDST = map[string][]dstPeriod{
`, tzdbVersion)
	for _, name := range names {
		zone := name
		if z, ok := db.links[name]; ok {
			zone = z
		}
		fmt.Fprintf(&b, "%q: %s,\n", name, varName(zone))
	}
	b.WriteString("}\n")
	for _, zone := range slices.Sorted(maps.Keys(data)) {
		fmt.Fprintf(&b, "\n// %s, tzdb %s rearguard.\nvar %s = []dstPeriod{\n", zone, tzdbVersion, varName(zone))
		for _, p := range data[zone] {
			if p.start == math.MinInt64 {
				fmt.Fprintf(&b, "{-1 << 63, %d, %v},\n", p.offset, p.dst)
				continue
			}

			fmt.Fprintf(&b, "{%d, %d, %v}, // %s\n", p.start, p.offset, p.dst, time.Unix(p.start, 0).UTC().Format(time.DateTime)+" UTC")
		}
		b.WriteString("}\n")
	}
	src, err := format.Source(b.Bytes())
	if err != nil {
		fail(err)
	}

	return src, names
}

// writeTestZones copies the two forms of testZones to dir, with the release
// in dir/version.
func writeTestZones(db *tzdb, dir string) {
	for _, form := range []string{db.main, db.rearguard} {
		for _, name := range testZones {
			b, err := os.ReadFile(filepath.Join(form, filepath.FromSlash(name)))
			if err != nil {
				fail(err)
			}

			dst := filepath.Join(dir, filepath.Base(form), filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				fail(err)
			}

			if err := os.WriteFile(dst, b, 0o644); err != nil {
				fail(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "version"), []byte(tzdbVersion+"\n"), 0o644); err != nil {
		fail(err)
	}
}
