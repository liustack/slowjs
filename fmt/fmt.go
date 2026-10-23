// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fmt exposes some functionality of the Go stdlib fmt package to a
// [slowjs.VM].
package fmt // import "github.com/liustack/slowjs/fmt"

import (
	"fmt"

	"github.com/liustack/slowjs"
)

const pkg = "fmt"

// Register sets the "fmt" property of 'obj' to the fmt package javascript object.
//
// The functions registered are
//
//   - [fmt.Printf]
//   - [fmt.Println]
//   - [fmt.Sprintf]
//   - [fmt.Sprintln]
//
// The javascript functions follow the usual conventions where the first letter
// of a function name is not capitalized. So [fmt.Println] becomes
// 'fmt.println' in javascript, for example.
func Register(obj slowjs.Value) (err error) {
	defer obj.Free()

	vm := obj.VM()
	mod, err := vm.EvalThisValue(obj.Dup(), fmt.Sprintf("this.%s={}; this.%[1]s", pkg), slowjs.EvalGlobal)
	if err != nil {
		return err
	}

	defer mod.Free()

	for _, v := range []struct {
		nm       string
		f        any
		wantThis bool
	}{
		{"printf", fmt.Printf, false},
		{"println", fmt.Println, false},
		{"sprint", fmt.Sprint, false},
		{"sprintf", fmt.Sprintf, false},
	} {
		longName := fmt.Sprintf("__%s_%s__", pkg, v.nm)
		if err := vm.RegisterFunc(longName, v.f, v.wantThis); err != nil {
			return err
		}

		if _, err := vm.EvalThis(mod.Dup(), fmt.Sprintf("this.%s = %s", v.nm, longName), slowjs.EvalGlobal); err != nil {
			return err
		}
	}

	return nil
}
