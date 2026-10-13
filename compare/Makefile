# Copyright 2024 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all clean edit editor work test cpu mem short-test benchmark cover

all: editor
	golint 2>&1
	staticcheck 2>&1

# The indirect requires here are only lower bounds; go mod tidy re-derives them
# from ../go.mod, which "replace github.com/liustack/quickjs => ../" makes authoritative.
# Without it a dependency bump in ../go.mod fails with "updates to go.mod needed".
benchmark:
	go mod tidy
	go test -vet=off -timeout 24h -run @ -bench . -benchtime 5s | go run benchreport.go

clean:
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile go.mod *.go & fi

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go install -v  ./...
	staticcheck

test:
	go test -failfast -v -timeout 24h -count=1 ./...
	grep -a 'TRC\|TODO\|ERRORF\|FAIL' log-test || true

short-test:
	go test -failfast -v -short -timeout 24h -count=1 ./...
	grep -a 'TRC\|TODO\|ERRORF\|FAIL' log-test || true

work:
	rm -f go.work*
	go work init
	go work use .
	if [ -d ../../libc ]; then go work use ../../libc; fi
	if [ -d ../../libquickjs ]; then go work use ../../libquickjs; fi

cpu: clean
	go test -run @ -bench BenchmarkArewefastyet/ccgo -cpuprofile cpu.out
	go tool pprof -web *.test cpu.out

mem: clean
	go test -run @ -bench BenchmarkArewefastyet/ccgo -memprofile mem.out -memprofilerate 1 -timeout 24h
	go tool pprof -lines -web -alloc_space *.test mem.out

cover:
	cover=$(shell mktemp) ; \
	      go test -v -short -coverprofile=$$cover ; \
	      go tool cover -html=$$cover
