# Copyright (c) 2026 The liustack/beastjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.

.PHONY: benchmark clean cpu mem

# benchmark times the scripts of testdata on the three engines, at least 5s
# each, and writes the table of benchreport.go. The output goes to a file
# first, not through a pipe, so that a failed go test stops here instead of
# making a table of half a run. go mod tidy first: the versions of go.mod are
# only a floor, ../go.mod decides.
benchmark:
	go mod tidy
	go test -vet=off -timeout 24h -run @ -bench . -benchtime 5s > bench.out; \
	status=$$?; \
	cat bench.out; \
	if [ $$status -ne 0 ]; then \
		echo "benchmark: go test failed with status $$status, no report"; \
		exit $$status; \
	fi; \
	go run benchreport.go < bench.out

clean:
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

cpu: clean
	go test -run @ -bench BenchmarkLIUSTACK -cpuprofile cpu.out
	go tool pprof -web *.test cpu.out

mem: clean
	go test -run @ -bench BenchmarkLIUSTACK -memprofile mem.out -memprofilerate 1 -timeout 24h
	go tool pprof -lines -web -alloc_space *.test mem.out
