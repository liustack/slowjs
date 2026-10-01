# Copyright 2024 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all build_all_targets clean edit editor work test cpu mem shorttest benchmark cover

all: editor
	golint 2>&1

benchmark:
	date
	make -C ./compare benchmark

build_all_targets:
	./build_all_targets.sh

clean:
	make -C ./compare/ clean
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile go.mod builder.json *.go & fi

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go build -o /dev/null generator.go
	go install -v  ./...
	staticcheck 2>&1

test:
	go test -failfast -v -timeout 24h -count=1 ./...

shorttest:
	go test -failfast -v -short -timeout 24h -count=1 ./...

work:
	rm -f go.work*
	go work init
	go work use .
	go work use ./compare
	if [ -d ../libc ]; then go work use ../libc; fi
	if [ -d ../libquickjs ]; then go work use ../libquickjs; fi

cpu:
	make -C ./compare cpu

mem:
	make -C ./compare mem

leak:
	go test -v -run TestMemgrind2 -tags=libc.memgrind ./...

cover:
	cover=$(shell mktemp) ; \
	      go test -v -short -coverprofile=$$cover ; \
	      go tool cover -html=$$cover
