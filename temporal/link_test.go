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

// A program that uses Temporal links the pack of the zones and calendar
// tables Temporal reads (tz.bin) and, since it imports package intl for
// toLocaleString, the packs intl links. It links none of the packs only
// go-intl's DateTimeFormat, zone names and guess at the host's zone read,
// which neither package uses (see internal/gointl/qjs_readers.go).
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
	const pkg = "github.com/liustack/quickjs/internal/gointl."
	for _, name := range []string{"qjsTZ", "qjsCore", "qjsNames", "qjsCJDict"} {
		if !linked[pkg+name] {
			t.Errorf("the program does not link %s", name)
		}
	}
	for _, name := range []string{"qjsDates", "qjsZoneNames", "qjsHostZones"} {
		if linked[pkg+name] {
			t.Errorf("the program links %s", name)
		}
	}
}
