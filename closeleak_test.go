// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// A Value that is never freed does not stop Close: the VM is released all the
// same.
func TestCloseWithValueLeft(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	m.GlobalObject() // never freed
	if _, err := m.EvalValue(`({a: [1, 2, 3]})`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`1 + 1`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.Close()
}
