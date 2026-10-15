// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func engineFile(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every inline property read of the interpreter gets a guard, whatever the
// name of its variables, and the result does not depend on the guards
// being there already.
func TestInterpreterGuardsEveryRead(t *testing.T) {
	b := engineFile(t)
	out, err := fixInterpreter(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, b) {
		t.Fatal("fixInterpreter changed a fixed file")
	}
	body, err := funcBody(out, reCallInternal, "_JS_CallInternal")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(reInterpGuard.FindAll(body, -1)); n != 3 {
		t.Fatalf("%d guards, want 3 (get_field, get_field2, get_length)", n)
	}
	for _, name := range []string{"get_field", "get_field2", "get_length"} {
		if !bytes.Contains(body, []byte(") {\n\t\t\t\t\tgoto "+name+"_slow_path\n\t\t\t\t}\n\t\t\t\t_ = ctx")) {
			t.Errorf("no guard in the %s read", name)
		}
	}
}

// A new property lookup in the interpreter that is neither a known inline
// read nor the put_field write stops fixlib.
func TestInterpreterRejectsUnknownLookup(t *testing.T) {
	b := engineFile(t)
	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(body, []byte("\n\tfor {"))
	if i < 0 {
		t.Fatal("no loop in _JS_CallInternal")
	}
	changed := append(append(append([]byte{}, body[:i]...), "\n\t_ = _find_own_property(tls, 0, 0, 0)"...), body[i:]...)
	_, err = fixInterpreter(bytes.Replace(b, body, changed, 1))
	if err == nil || !strings.Contains(err.Error(), "review the new one") {
		t.Fatalf("got %v, want a request to review the new lookup", err)
	}
}

// A function that reaches property values and is not in slotSites, or that
// reaches them more often than slotSites says, stops fixlib.
func TestAuditSlotsRejectsChanges(t *testing.T) {
	b := engineFile(t)
	if err := auditSlots(b); err != nil {
		t.Fatal(err)
	}

	added := append(append([]byte{}, b...), "\nfunc _new_reader(tls *libc.TLS, p uintptr) {\n\t_find_own_property(tls, 0, p, 0)\n}\n"...)
	if err := auditSlots(added); err == nil || !strings.Contains(err.Error(), "_new_reader reaches property values and is not listed") {
		t.Fatalf("new function: got %v", err)
	}

	body, err := funcBody(b, reGetPropString, "_get_prop_string")
	if err != nil {
		t.Fatal(err)
	}
	grown := bytes.Replace(b, body, append(body[:len(body)-2:len(body)-2], "\t_ = (*TJSObject)(unsafe.Pointer(p)).Fprop\n}\n"...), 1)
	if err := auditSlots(grown); err == nil || !strings.Contains(err.Error(), "_get_prop_string: find, prop, own, pr are 2, 1, 0, 0") {
		t.Fatalf("changed function: got %v", err)
	}
}
