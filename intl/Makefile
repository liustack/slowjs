# Copyright 2026 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all clean edit editor leak test

all: editor
	golint 2>&1

clean:
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile *.go *.js & fi

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go install -v  ./...
	staticcheck 2>&1

leak:
	go test -v -run TestMemgrind2 -tags=libc.memgrind

test:
	go test -failfast -v -timeout 24h -count=1 ./...
