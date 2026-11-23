# Copyright (c) 2026 The liustack/tifajs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE at the
# root of this repository.

# The targets are described in docs/internals/testing.md (Makefile targets).

.PHONY: benchmark bench build_all_targets check-fast check-full check-gen \
	clean compare-engines cover cpu editor gen gointl inject-goldens mem \
	remote shorttest test work

# make gen, or make gen ARCH=arm64: the engine as Go assembly.
ARCH ?= amd64

# make inject-goldens INJECT_JOBS=8 INJECT_PATHS="...": the paths of the
# scripts and how many run at once.
INJECT_JOBS ?= 4
INJECT_PATHS ?= engine/test/regvm/corpus engine/test/regvm/js

# make remote RUN="check-fast" REF=HEAD TREE=main REMOTE_FLAGS="...": the
# target to run on another machine, on the commit REF over the tree TREE.
REF ?= HEAD
TREE ?= main

benchmark:
	date
	make -C ./compare benchmark

# make compare-engines ARGS="...": the arguments of cmd/compare.
compare-engines:
	cd compare/engines && go run ./cmd/compare $(ARGS)

gen:
	scripts/gen-engine.sh $(ARCH)

check-fast:
	scripts/check.sh fast

check-full:
	scripts/check.sh full

check-gen:
	scripts/check-gen.sh

# make bench ARGS="...": the arguments of go tool measure bench.
bench:
	go tool measure bench $(ARGS)

inject-goldens:
	EXTRA_DEFS=-DRV_INJECT engine/test/build.sh plain engine/test/out/inject-plain
	EXTRA_DEFS=-DRV_INJECT engine/test/regvm/build.sh plain engine/test/out/inject-plain
	engine/test/regvm/inject.sh -update -j $(INJECT_JOBS) engine/test/out/inject-plain $(INJECT_PATHS)

remote:
	scripts/remote.sh -t $(TREE) $(REMOTE_FLAGS) $(REF) -- make $(RUN)

build_all_targets:
	./build_all_targets.sh

clean:
	make -C ./compare/ clean
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go install -v ./...
	staticcheck

test:
	go test -failfast -v -timeout 24h -count=1 ./...

shorttest:
	go test -failfast -v -timeout 24h -count=1 -short ./...

gointl:
	go test -count=1 -tags gointl_walk ./internal/gointl/internal/...
	go test -count=1 -tags tifajs_intl_nonames,tifajs_intl_nocjdict ./internal/gointl/...
	go test -count=1 -tags tifajs_intl_nonames,tifajs_intl_nocjdict -run WithoutNames ./intl/
	internal/gointl/data.sh checkout
	internal/gointl/data.sh pack -check
	internal/gointl/data.sh test -count=1 -p 2

work:
	rm -f go.work*
	go work init
	go work use .
	go work use ./compare

cpu:
	make -C ./compare cpu

mem:
	make -C ./compare mem

cover:
	t=$$(mktemp) && go test -v -short -coverprofile=$$t . && go tool cover -html=$$t && rm -f $$t
