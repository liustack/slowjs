# Copyright 2025 The libquickjs-go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

.PHONY:	all bench clean cpu edit editor generate download dev test nuke shorttest \
	w64 dev-w64 grep ktest test2 test3 dbg

QJS_ZIP = 3d5e064e9dd67c70f7962836505a7fa067bf0a4e.zip
QJS_URL = https://github.com/bellard/quickjs/archive/$(QJS_ZIP)

TEST262_ZIP = 5c8206929d81b2d3d727ca6aac56c18358c8d790.zip
TEST262_URL = https://github.com/tc39/test262/archive/$(TEST262_ZIP)

GREP = 'TRC\|TODO\|ERRORF\|FAIL\|undefined:'

all: editor
	golint 2>&1
	staticcheck 2>&1

bench:
	go test -v -timeout 24h -run @ -bench .

clean:
	rm -f log-* cpu.test mem.test *.out *.exe tmp_file.txt
	go clean

cpu: clean
	go test -run @ -bench . -cpuprofile cpu.out
	go tool pprof -lines *.test cpu.out

dev: download
	echo -n > /tmp/ccgo.log
	GO_GENERATE_DEV=1 go run -tags=ccgo.dmesg generator.go
	gofmt -l -s -w .
	go build -v -o /dev/null .
	go build -v -o /dev/null ./internal/run-test262
	grep $(GREP) /tmp/ccgo.log || true
	git status

dev-w64: download
	echo -n > /tmp/ccgo.log
	GO_GENERATE_DEV=1 GO_GENERATE_W64=1 go run -tags=ccgo.dmesg generator.go
	gofmt -l -s -w .
	GOOS=windows GOARCH=amd64 go build -v -o /dev/null .
	GOOS=windows GOARCH=amd64 go build -v -o /dev/null ./internal/run-test262
	grep $(GREP) /tmp/ccgo.log || true
	git status

download:
	@if [ ! -f $(QJS_ZIP) ]; then wget $(QJS_URL) ; fi
	@if [ ! -f $(TEST262_ZIP) ]; then wget $(TEST262_URL) ; fi

edit:
	@touch log
	@if [ -f "Session.vim" ]; then gvim -S & else gvim -p Makefile go.mod builder.json all_test.go generator.go libquickjs.go & fi

editor:
	@gofmt -l -s -w $(shell find -name \*.go | grep -v o.go)
	go test -vet=off -c -o /dev/null
	go build -o /dev/null generator*.go

generate: download
	go generate
	gofmt -l -s -w .
	go build -v -o /dev/null .
	go build -v -o /dev/null ./internal/run-test262
	git status

grep:
	grep -n $(GREP) /tmp/ccgo.log || true

nuke: clean
	rm -rf ./tmp/ ./test262/

shorttest:
	go test -vet=off -v -timeout 24h -short

test:
	go test -vet=off -v -timeout 24h -failfast

test2:
	go test -vet=off -v -timeout 24h -failfast -run Test2$$

test3:
	go test -vet=off -v -timeout 24h -failfast -run Test3$$

ktest:
	go test -vet=off -v -timeout 24h -failfast -keep

w64: download
	GO_GENERATE_W64=1 go run -tags=ccgo.dmesg generator.go
	gofmt -l -s -w .
	GOOS=windows GOARCH=amd64 go build -v -o /dev/null .
	GOOS=windows GOARCH=amd64 go build -v -o /dev/null ./internal/run-test262
	git status

dbg:
	go test -timeout 24h -run TestIssue6$$
