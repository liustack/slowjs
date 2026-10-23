// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Temporal and its Intl formatting link the date and zone-name packs, but
// never the tables used to guess the host's zone.
func TestLinksOnlyReadData(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	bin := filepath.Join(t.TempDir(), "linkprobe")
	if out, err := exec.Command("go", "build", "-o", bin, "./testdata/linkprobe").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if out, err := exec.Command(bin).CombinedOutput(); err != nil {
		t.Fatalf("linkprobe: %v\n%s", err, out)
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
	const pkg = "github.com/liustack/slowjs/internal/gointl."
	for _, name := range []string{"qjsTZ", "qjsCore", "qjsNames", "qjsCJDict", "qjsDates", "qjsZoneNames"} {
		if !linked[pkg+name] {
			t.Errorf("the program does not link %s", name)
		}
	}
	for _, name := range []string{"qjsHostZones"} {
		if linked[pkg+name] {
			t.Errorf("the program links %s", name)
		}
	}
}
