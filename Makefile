# Copyright 2024 The quickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all build_all_targets clean edit editor work test cpu mem shorttest benchmark compare-engines cover gointl gen check-gen check-fast check-full bench remote inject-goldens

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

# The C engine of engine/ as Go assembly in internal/cengine, on a machine with
# LLVM 22.1.8: make gen, make gen ARCH=arm64. GEN_KEEP=dir keeps a copy of the
# generated files (see docs/testing.md).
ARCH ?= amd64
gen:
	scripts/gen-engine.sh $(ARCH)

# The checks of a change to the C engine, one table at the end: fast for every
# commit, full for the end of a batch. The steps are in scripts/check.sh.
check-fast:
	scripts/check.sh fast

check-full:
	scripts/check.sh full

# The generated engine checked in under internal/cengine is what make gen makes
# now, for both architectures (scripts/check-gen.sh, part of check-full).
check-gen:
	scripts/check-gen.sh

# Code-position timing and callgrind (internal/measure), for example
# make bench ARGS="report -in shop.jsonl -base base -new new".
bench:
	go tool measure bench $(ARGS)

# The goldens of the throw injection of the register loop written again from a
# run of every place, with the places and keys that changed
# (engine/test/regvm/inject.sh -update). About 10 hours on one core: run it on
# a server, make remote RUN=inject-goldens with -g engine/test/regvm/golden/inject
# in REMOTE_FLAGS to bring the goldens back. INJECT_PATHS names fewer scripts.
INJECT_JOBS ?= 4
INJECT_PATHS ?= engine/test/regvm/corpus engine/test/regvm/js
inject-goldens:
	EXTRA_DEFS=-DRV_INJECT engine/test/build.sh plain engine/test/out/inject-plain
	EXTRA_DEFS=-DRV_INJECT engine/test/regvm/build.sh plain engine/test/out/inject-plain
	engine/test/regvm/inject.sh -update -j $(INJECT_JOBS) engine/test/out/inject-plain $(INJECT_PATHS)

# A make target on another machine (scripts/remote.sh): make remote RUN="gen",
# make remote REF=my-branch TREE=new RUN="check-fast". The machine comes from
# SLOWJS_REMOTE and SLOWJS_REMOTE_DIR, the cores from SLOWJS_REMOTE_CPUS.
REF ?= HEAD
TREE ?= main
remote:
	scripts/remote.sh -t $(TREE) $(REMOTE_FLAGS) $(REF) -- make $(RUN)

build_all_targets:
	./build_all_targets.sh

clean:
	make -C ./compare/ clean
	rm -f log-* cpu.test mem.test *.out go.work*
	go clean

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile go.mod *.go & fi

editor:
	gofmt -l -s -w .
	go test -c -o /dev/null ./...
	go install -v  ./...
	staticcheck 2>&1

test:
	go test -failfast -v -timeout 24h -count=1 ./...

shorttest:
	go test -failfast -v -short -timeout 24h -count=1 ./...

# The checks of internal/gointl that go test ./... does not run: the pack
# tool's tests, which need its build tag, the build without the optional
# packs, there and in intl, that qjsdata/ is what go-intl's data gives, and
# the tests of every language against Node's answers that came with
# go-intl (which clones go-intl once, and reads it from the cache after).
gointl:
	go test -count=1 -tags gointl_walk ./internal/gointl/internal/...
	go test -count=1 -tags slowjs_intl_nonames,slowjs_intl_nocjdict ./internal/gointl/...
	go test -count=1 -tags slowjs_intl_nonames,slowjs_intl_nocjdict -run WithoutNames ./intl/
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
	cover=$(shell mktemp) ; \
	      go test -v -short -coverprofile=$$cover ; \
	      go tool cover -html=$$cover
