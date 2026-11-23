// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs_test

import (
	"fmt"

	"github.com/liustack/tifajs"
)

// pipe is a VM with a send and a receive over two Go channels.
func pipe(out chan<- string, in <-chan string) *tifajs.VM {
	m := newVM()
	if err := m.RegisterFunc("send", func(s string) { out <- s }, false); err != nil {
		panic(err)
	}
	if err := m.RegisterFunc("receive", func() string { return <-in }, false); err != nil {
		panic(err)
	}
	return m
}

// Two VMs run at once, each on a goroutine of its own, and talk over Go
// channels. A VM is used by one goroutine at a time, and VMs share nothing.
func Example_ping() {
	toServer := make(chan string, 1)
	toClient := make(chan string, 1)

	go func() {
		server := pipe(toClient, toServer)
		defer server.Close()
		if _, err := server.Eval(`send(receive() + ' reply')`, tifajs.EvalGlobal); err != nil {
			panic(err)
		}
	}()

	client := pipe(toServer, toClient)
	defer client.Close()
	fmt.Println(client.Eval(`send('ping'); receive();`, tifajs.EvalGlobal))
	// Output:
	// ping reply <nil>
}
