# Copyright 2025 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all clean edit editor test

all: editor
	golint 2>&1

clean:
	make -C ./compare/ clean
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile *.go & fi

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go install -v  ./...
	staticcheck 2>&1

test:
	go test -failfast -v -timeout 24h -count=1 ./...
