# Copyright 2024 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all build_all_targets clean edit editor work test cpu mem shorttest benchmark compare-engines cover gointl

all: editor
	golint 2>&1

benchmark:
	date
	make -C ./compare benchmark

# Compares this engine with the other pure Go JavaScript engines and writes
# compare/engines/results/<date>-<version>-<host>.{json,md}. See
# docs/benchmarks.md. Pass flags with ARGS, for example
# make compare-engines ARGS="-cpu 1 -sibling 5".
compare-engines:
	cd compare/engines && go run ./cmd/compare $(ARGS)

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

# The checks of internal/gointl that go test ./... does not run: the pack
# tool's tests, which need its build tag, the build without the optional
# packs, there and in intl, and that qjsdata/ is what go-intl's data gives
# (which clones go-intl).
gointl:
	go test -count=1 -tags gointl_walk ./internal/gointl/internal/...
	go test -count=1 -tags quickjs_intl_nonames,quickjs_intl_nocjdict ./internal/gointl/...
	go test -count=1 -tags quickjs_intl_nonames,quickjs_intl_nocjdict -run WithoutNames ./intl/
	internal/gointl/qjs-sync.sh checkout
	internal/gointl/qjs-sync.sh pack -check

work:
	rm -f go.work*
	go work init
	go work use .
	go work use ./compare
	if [ -d ../libc ]; then go work use ../libc; fi

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
