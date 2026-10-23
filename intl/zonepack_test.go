// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A fresh process avoids time.LoadLocation's process-wide ZONEINFO cache.
// The fake file is a valid TZif zone that claims Winnipeg is always UTC.
func TestEmbeddedZonePack(t *testing.T) {
	if os.Getenv("QUICKJS_ZONEPACK_CHILD") == "1" {
		vm := newVM(t)
		got := goldenEval(t, vm, `new Intl.DateTimeFormat('en',{timeZone:'America/Winnipeg',hour:'2-digit',hourCycle:'h23',minute:'2-digit'}).format(Date.UTC(2026,0,15,12))`)
		if got != "06:00" {
			t.Fatalf("embedded Winnipeg zone: got %q, want 06:00", got)
		}
		return
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "America"), 0700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 44)
	copy(data, "TZif")
	binary.BigEndian.PutUint32(data[36:], 1)
	binary.BigEndian.PutUint32(data[40:], 4)
	data = append(data, 0, 0, 0, 0, 0, 0, 'U', 'T', 'C', 0)
	if err := os.WriteFile(filepath.Join(dir, "America", "Winnipeg"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestEmbeddedZonePack$")
	cmd.Env = append(os.Environ(), "QUICKJS_ZONEPACK_CHILD=1", "ZONEINFO="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
