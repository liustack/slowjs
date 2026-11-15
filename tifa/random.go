// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package tifa

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math"

	"github.com/liustack/slowjs"
)

// registerRandom defines __tifa_random_bytes__(n), n bytes of crypto/rand in base64, n an integer
// from 0 to 65536. The bytes are read on each call, including calls made while the bundle loads:
// there is no pool of random bytes to keep per VM or to reuse between renders by mistake. Moved
// here from internal/ssr/random.go of tifa.
func registerRandom(vm *slowjs.VM) error {
	return vm.RegisterHostFunc("__tifa_random_bytes__", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("random bytes requires a length")
		}
		n, ok := args[0].(float64)
		if i, integral := args[0].(int); integral {
			n, ok = float64(i), true
		}
		if !ok || n < 0 || n > 65536 || n != math.Trunc(n) {
			return nil, errors.New("random byte length must be an integer from 0 to 65536")
		}
		buf := make([]byte, int(n))
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		return base64.StdEncoding.EncodeToString(buf), nil
	})
}
