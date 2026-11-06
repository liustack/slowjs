# liustack/quickjs (project notes for AI agents)

## Goal

`github.com/liustack/quickjs` is a pure Go JavaScript engine from QuickJS, built and kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages to HTML inside the Go process. From v0.28.0 to v0.29.11 it was named `github.com/liustack/slowjs`, and those versions stay at that path. The Go API at the root started from `modernc.org/quickjs`. The engine is QuickJS in C (`engine/`), compiled by LLVM to Go assembly that is checked in, so building needs no C compiler. Until v0.28 the engine was QuickJS translated to Go by ccgo, and that engine is gone. liustack/quickjs makes renders faster, makes limits hold (a timeout or running out of memory ends the call whatever the script does), and makes output the same on every machine.

## Hard constraints

- Pure Go, `CGO_ENABLED=0`. No cgo, no C compiler, no shared library for the user of the module. The module runs on linux and darwin, on amd64 and arm64, and on windows/amd64, only, and a build for another target stops with the error of `internal/unsupported`. Do not bring in a cgo engine.
- The generated C engine in `internal/cengine` (the `.s`, `engine_decl_*.go` and `engine_keep_*.go` files of amd64 and arm64) is checked in, never edited by hand, and always what `make gen` and `make gen ARCH=arm64` make from `engine/` now. A commit that changes the C of `engine/`, the generator under `internal/goasm/gen`, the pass, or the flags of `scripts/gen-engine.sh` carries the files made again, in that same commit. `make check-gen` (part of `make check-full`) fails when they differ. Generate on a machine with LLVM 22.1.8 (`make remote` runs it on another machine, see [docs/testing.md](docs/testing.md#on-another-machine)).
- The C is not edited to look like upstream. It is QuickJS at the pinned commit with the changes of liustack/quickjs ([docs/changes](docs/changes/README.md)), and a commit of upstream QuickJS is taken into it one at a time, by hand, as [docs/upgrading.md](docs/upgrading.md) says.
- A change for speed leaves what the engine gives the same: the same HTML of tifa's pages, the same known test262 errors (33) (`engine/test/test262_errors.txt`), and the same call depth, so a script recurses exactly as deep as before.

## Current state of the repository

- `main` is the branch of liustack/quickjs. Every release since v0.26.0 is a lightweight tag on a merge commit of `main`.
- Root: the Go API, package `quickjs` (`quickjs.go` and the files next to it), and its tests. It calls the engine only through `engine.go`, `entry.go`, `hostcall.go` and `enginememory.go` (see [docs/architecture.md](docs/architecture.md#the-engine-layer)).
- `engine/`: the C tree of the engine, QuickJS at the pinned commit with the C changes of liustack/quickjs. It has no Go file, and no Go package directory may hold a C file (`layout_test.go` checks it). It builds with `-ffreestanding -nostdinc` against `engine/libc`, its own small libc (musl's string and math functions, see [engine/libc/README.md](engine/libc/README.md)), and asks its host only for the callbacks of `engine/host.h`. `engine/test/` is the native test host that runs test262 and the recorded outputs on it, also under the sanitizers (see [docs/testing.md](docs/testing.md#the-c-tree-and-its-native-test-host)).
- `internal/cengine/`: the generated engine and the Go host files next to it. `internal/goasm/gen/`: the generator that compiles `engine/` to Go assembly (see its README), and `internal/goasm/cmd/xform` that moves the code for timing. `internal/gohost/`, `internal/hostmem/`, `internal/idtable/`: the Go side of the host callbacks, the memory of a VM and the table of handles. `internal/measure/`: the tool that times builds, `go tool measure`.
- `intl/`: `Intl` for a VM, in every locale Chromium has. `temporal/`: `Temporal` for a VM, computed by go-intl. `internal/gointl/`: a copy of go-intl, which `intl/` and `temporal/` use. `internal/intlhook/`: what `temporal/` takes from `intl/`. `fmt/`: Go's fmt for a VM. `compare/`: a module of its own, with QuickJS's micro benchmarks. `compare/engines/`: another module, the comparison with the other pure Go engines that docs/benchmarks.md reports.
- `assets/`: the logo, icon, banner and share card in `assets/brand/` (the gopher drawn after Renée French's Go gopher, CC BY 4.0, see NOTICE.md) and the WeChat QR code the READMEs show.
- [docs/architecture.md](docs/architecture.md) has the full map, the core types and the rules of the Go API.

## Docs (`docs/`)

1. Every doc starts with front matter: `summary` (one sentence saying what question the doc answers) and `read_when` (when to read it). See the comment at the top of [scripts/docs-list.sh](scripts/docs-list.sh) for the format.
2. Before starting work and before creating a doc, run `scripts/docs-list.sh` to see the index, and read the docs whose Read when applies. Before committing, run `scripts/docs-list.sh --check`. It fails if a doc has no summary.
3. What each doc is for:
   - [docs/architecture.md](docs/architecture.md): the layers, folders, core types and rules of the Go API
   - [docs/changes/](docs/changes/README.md): every change liustack/quickjs makes to the QuickJS C tree and to the Go API, by area
   - [docs/testing.md](docs/testing.md), [docs/commit.md](docs/commit.md): checks and commit rules
   - [docs/allocator.md](docs/allocator.md): the memory allocator of the C engine in `engine/`
   - [docs/upgrading.md](docs/upgrading.md): taking commits of upstream QuickJS and new intl data
   - [docs/performance.md](docs/performance.md): render speed of each release, measured in tifa
   - [docs/measuring.md](docs/measuring.md): how to time a change and judge it, the defaults and what they can see
   - docs/benchmarks.md: the comparison with the other pure Go JavaScript engines, with its history, run again at every minor release, and at a patch release when the owner asks. It leaves out goja, sobek and otto. Its results are kept under `compare/engines/results/` and its changes in `compare/engines/CHANGELOG.md`
   - [docs/releasing.md](docs/releasing.md): versions, tags and what follows a release
   - [docs/upstream.md](docs/upstream.md): where the code comes from, how QuickJS commits are taken in, and what changes when moving from modernc.org/quickjs
   - [docs/api.md](docs/api.md): the Go API beyond Eval and Call, with the release of each function
4. Each change liustack/quickjs makes to the QuickJS C tree or to the Go API is written up in the doc of its area under [docs/changes](docs/changes/README.md), with a row in its index: what it does, why it is safe, what it gains or costs and how that was measured, its tests. A change for speed also goes into [docs/performance.md](docs/performance.md).
   - New changes are written into [docs/changes](docs/changes/README.md), never into README.md. The README is a landing page: it only links to the docs. New Go API goes into [docs/api.md](docs/api.md), in both languages, with the release it came in.
   - A branch started before the README became a landing page may still add its notes to the old long README. When it is merged, move each note to where it belongs: the change into the doc of its area under docs/changes with a row in the index ("Unreleased" until it ships), its focused checks into [docs/testing.md](docs/testing.md), its numbers into [docs/performance.md](docs/performance.md), and what users see into the Unreleased section of both changelogs. Then check that each paragraph the branch added to the README has a place in the new docs.

   A claim about how this engine ranks among the other JavaScript engines for Go, in the README or elsewhere, needs a measurement in docs/benchmarks.md first.
5. Docs are written for readers: write conclusions and how to do things, not reasoning or work logs.
6. Every doc is written in English and has a Chinese version `X.zh-CN.md` next to it: every doc of `docs/`, README.md, CHANGELOG.md and the READMEs and changelogs listed in `scripts/docs-list.sh`. When you change one side, change the other in the same commit. Under the title, each links to the other (`English · [简体中文](X.zh-CN.md)`). A Chinese doc links to the Chinese version of another doc, with the anchor GitHub makes from its Chinese heading. `scripts/docs-list.sh --check` fails when a doc has no Chinese version.
7. English: keep it short and direct, use everyday words, and do not invent terms. Chinese: do not use semicolons, and do not use dashes as punctuation.
8. Code comments and error messages point to a doc by its path, such as `docs/upgrading.md`, never to a section of the README.

## Issues and pull requests

liustack/quickjs does not accept pull requests: [CONTRIBUTING.md](CONTRIBUTING.md) says so, and `.github/PULL_REQUEST_TEMPLATE.md` tells whoever opens one. Bugs and ideas come in as issues through the templates in `.github/ISSUE_TEMPLATE/`, and security problems as private reports ([SECURITY.md](SECURITY.md)). Do not merge a pull request, and keep these files the same in spirit as those of liustack/modlens and tifa.

## License headers

- A new file written in liustack/quickjs starts with this header, in the comment syntax of its language and with the year it was created:

  ```go
  // Copyright (c) 2026 The liustack/quickjs Authors.
  // SPDX-License-Identifier: MIT
  // Use of this source code is governed by the MIT license in LICENSE-MIT at the
  // root of this repository.
  ```

- A file that came from upstream (QuickJS, modernc.org/quickjs, go-intl), a generated file, and a new file made from one of them keep the upstream header and license, however much liustack/quickjs changes them.
- Add a new author of liustack/quickjs to `AUTHORS-LIUSTACK`. `AUTHORS` lists the authors of modernc.org/quickjs and stays as that project has it.
- New code or data from a third party keeps its license file next to it and gets a row in [NOTICE.md](NOTICE.md). Say MIT only together with what stays under the upstream licenses.

## Draft folder

Plans, design drafts, experiment notes and raw measurement data go in the repository draft folder `.issues/<YYYY-MM-DD-topic>/`, with the files for one task in one folder. `.issues/` is not checked in (add it to `.git/info/exclude` in a fresh clone), and checked-in code, docs and scripts must not refer to anything in it. Conclusions that are settled and need to be kept long term are cleaned up and written into `docs/`.

## Verification and commits

- See [docs/testing.md](docs/testing.md) for verification commands and [docs/commit.md](docs/commit.md) for commit rules. Every commit passes `go build ./...`, `go vet ./...` and `scripts/docs-list.sh --check`, and `go test -count=1 -p 1 .` (about a minute). The branch passes `make check-full` before it is merged.
- The generated files are large. Build and test a few packages at a time (`-p 2`), and run heavy jobs such as test262, `make build_all_targets` or the race detector one after another, not in parallel.
- For new behavior, write a failing test first, then implement. For a bug, find the root cause before changing code, and do not hide it with fallback values.
- After a new release of QuickJS, follow [docs/upgrading.md](docs/upgrading.md).
- Tools are Go programs or short shell scripts. Do not add Python.
- When merging a branch that changes what programs using the module can see, add it to both CHANGELOG.md and CHANGELOG.zh-CN.md.

## Releases

See [docs/releasing.md](docs/releasing.md).

- Release patch versions only (v0.26.17, v0.26.18, ...). Do not move to a new minor version for a small problem.
- A release is a lightweight tag on the merge commit of `main`. There are no GitHub releases.
- Ask the user before you create a tag, and again before you push one. Never push `main` or a tag on your own.
- After a minor release (or a patch release the owner names), run the comparison with the other engines again and update docs/benchmarks.md as docs/releasing.md says. After every release, upgrade tifa to it and measure the release on tifa's pages.
