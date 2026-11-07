# Contributing to liustack/quickjs

English · [简体中文](CONTRIBUTING.zh-CN.md)

First, the policy: **liustack/quickjs does not accept pull requests.** One maintainer reviews and owns every line of the engine, the generator and the Go API, and keeping that loop tight is what keeps it dependable.

Two contributions that help:

- **[Open an issue](https://github.com/liustack/quickjs/issues/new/choose).** A script that gives the wrong answer, a render that is slow, a limit that did not hold, an error that reads wrong, a doc that is unclear. Issues are read and decide what gets built next. The templates say what to include. English or Chinese is fine.
- **Fork it.** The files liustack/quickjs created are under the MIT license, and the code from other projects keeps its own license ([NOTICE.md](NOTICE.md)). Your copy is yours to change and publish.

A security problem goes in a private report, not an issue: see [SECURITY.md](SECURITY.md).

Everything below is for people working on a fork.

## Scope

liustack/quickjs is a JavaScript engine for Go programs, kept for [tifa](https://github.com/liustack/tifa), which renders React pages to HTML inside the Go process. It stays:

- pure Go, built with `CGO_ENABLED=0`, with no C compiler and no shared library for the user of the module
- an engine, not Node: no `require`, `fs`, `process` or event loop of its own. A host gives a script what it needs through the Go API
- QuickJS at a pinned commit with the changes written up in [docs/changes](docs/changes/README.md). Commits of upstream QuickJS are taken in one at a time ([docs/upgrading.md](docs/upgrading.md))
- the same output on every machine, and a change for speed leaves what the engine gives the same

## Setup

```sh
go build ./...
go vet ./...
scripts/docs-list.sh --check
go test -count=1 -p 1 .   # about a minute
```

Go 1.26.6 or later. Making the engine again from the C of `engine/` (`make gen`) needs LLVM 22.1.8, and `make check-full` runs every check, test262 and the sanitizers among them. [docs/testing.md](docs/testing.md) has all of it.

## Tests

- New behavior starts with a failing test, and a bug fix comes with a test that shows the bug.
- A change to the C of `engine/` comes with the generated files of `internal/cengine` made again, in the same commit. `make check-gen` fails when they differ.
- test262 keeps its 33 known errors (`engine/test/test262_errors.txt`).

## Commits

- `type(scope): imperative summary`, at most 72 characters, no period at the end.
- One commit does one thing, and the tree builds and passes its tests after each one.

Full rules are in [docs/commit.md](docs/commit.md). The map of the code is in [docs/architecture.md](docs/architecture.md), and the rules for working on it are in [AGENTS.md](AGENTS.md).
