// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A program that uses all of package intl links the go-intl packs its code
// reads and none of those only other readers read: DateTimeFormat, the zone
// names and the guess at the host's zone are not go-intl's here, so their
// 2.8 MB of data stay out (see internal/gointl/qjs_readers.go). A program
// that formats a date with go-intl links dates.bin and zonenames.bin.
func TestLinksOnlyReadData(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	for _, probe := range []struct {
		dir          string
		links, skips []string
	}{
		{"linkprobe", []string{"qjsCore", "qjsNames", "qjsCJDict"}, []string{"qjsDates", "qjsZoneNames", "qjsHostZones"}},
		{"linkprobedtf", []string{"qjsCore", "qjsDates", "qjsZoneNames"}, []string{"qjsHostZones"}},
	} {
		linked := linkedSymbols(t, probe.dir)
		const pkg = "github.com/liustack/quickjs/internal/gointl."
		for _, name := range probe.links {
			if !linked[pkg+name] {
				t.Errorf("%s does not link %s", probe.dir, name)
			}
		}
		for _, name := range probe.skips {
			if linked[pkg+name] {
				t.Errorf("%s links %s", probe.dir, name)
			}
		}
	}
}

// linkedSymbols builds and runs the program in testdata/dir and returns the
// symbols it links.
func linkedSymbols(t *testing.T, dir string) map[string]bool {
	bin := filepath.Join(t.TempDir(), dir)
	if out, err := exec.Command("go", "build", "-o", bin, "./testdata/"+dir).CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", dir, err, out)
	}
	out, err := exec.Command("go", "tool", "nm", bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	linked := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			linked[f[len(f)-1]] = true
		}
	}
	return linked
}
