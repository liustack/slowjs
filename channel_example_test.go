// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "modernc.org/quickjs"

import (
	"fmt"
)

// Multiple concurrent Javascript virtual machines communicating via Go channels.
func Example_ping() {
	tx := make(chan string, 1)
	rx := make(chan string, 1)
	client, _ := NewVM()
	defer client.Close()
	registerFuncs(client, tx, rx)
	go func() { // Start the server.
		server, _ := NewVM()
		defer server.Close()
		registerFuncs(server, rx, tx)
		server.Eval("send(receive()+' reply');", EvalGlobal)
	}()
	fmt.Println(client.Eval("send('ping'); receive();", EvalGlobal)) // Ping the server.
	// Output:
	// ping reply <nil>
}

func registerFuncs(m *VM, tx, rx chan string) {
	m.RegisterFunc("send", func(s string) { tx <- s }, false)
	m.RegisterFunc("receive", func() string { return <-rx }, false)
}
