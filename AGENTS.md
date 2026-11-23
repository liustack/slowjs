# liustack/tifajs (project notes for AI agents)

## Goal

`github.com/liustack/tifajs` is a pure Go JavaScript engine from QuickJS, built and kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages to HTML inside the Go process. It was named `github.com/liustack/slowjs` from v0.28.0 to v0.29.11 and from v0.31.0 to v0.31.12, `github.com/liustack/quickjs` from v0.30.0 to v0.30.7, and is `github.com/liustack/tifajs` from v0.32.0. Those versions stay at their old paths. The Go API at the root started from `modernc.org/quickjs`, and its code has since been written anew for liustack/tifajs. The engine is QuickJS in C (`engine/`), compiled by LLVM to Go assembly that is checked in, so building needs no C compiler. Until v0.28 the engine was QuickJS translated to Go by ccgo, and that engine is gone. liustack/tifajs makes renders faster, makes limits hold (a timeout or running out of memory ends the call whatever the script does), and makes output the same on every machine.

## Hard constraints

- Pure Go, `CGO_ENABLED=0`. No cgo, no C compiler, no shared library for the user of the module. The module runs on linux, darwin and windows, on amd64 and arm64, only, and a build for another target stops with the error of `internal/unsupported`. Do not bring in a cgo engine.
- The generated C engine in `internal/cengine` (the `.s`, `engine_decl_*.go` and `engine_keep_*.go` files of amd64 and arm64) is checked in, never edited by hand, and always what `make gen` and `make gen ARCH=arm64` make from `engine/` now. A commit that changes the C of `engine/`, the generator under `internal/goasm/gen`, the pass, or the flags of `scripts/gen-engine.sh` carries the files made again, in that same commit. `make check-gen` (part of `make check-full`) fails when they differ. Generate on a machine with LLVM 22.1.8 (`make remote` runs it on another machine, see [docs/internals/testing.md](docs/internals/testing.md#on-another-machine)).
- The C is not edited to look like upstream. It is QuickJS at the pinned commit with the changes of liustack/tifajs ([docs/internals/changes](docs/internals/changes/README.md)), and a commit of upstream QuickJS is taken into it one at a time, by hand, as [docs/internals/upgrading.md](docs/internals/upgrading.md) says.
- A change for speed leaves what the engine gives the same: the same HTML of tifa's pages, the same known test262 errors (33) (`engine/test/test262_errors.txt`), and a call depth that never gets less: a script recurses at least as deep as before, and may recurse deeper.

## Current state of the repository

- `main` is the branch of liustack/tifajs. Every release since v0.26.0 is a lightweight tag on a merge commit of `main`. From v0.31.0, pushing the tag also makes a GitHub release of it, which only carries the prebuilt binaries of the `tifajs` command.
- Root: the Go API, package `tifajs` (`tifajs.go` and the files next to it), and its tests. It calls the engine only through `engine.go`, `entry.go`, `hostcall.go` and `enginememory.go` (see [docs/internals/architecture.md](docs/internals/architecture.md#the-engine-layer)).
- `engine/`: the C tree of the engine, QuickJS at the pinned commit with the C changes of liustack/tifajs. It has no Go file, and no Go package directory may hold a C file (`layout_test.go` checks it). It builds with `-ffreestanding -nostdinc` against `engine/libc`, its own small libc (musl's string and math functions, see [engine/libc/README.md](engine/libc/README.md)), and asks its host only for the callbacks of `engine/host.h`. `engine/test/` is the native test host that runs test262 and the recorded outputs on it, also under the sanitizers (see [docs/internals/testing.md](docs/internals/testing.md#the-c-tree-and-its-native-test-host)).
- `internal/cengine/`: the generated engine and the Go host files next to it. `internal/goasm/gen/`: the generator that compiles `engine/` to Go assembly (see its README), and `internal/goasm/cmd/xform` that moves the code for timing. `internal/gohost/`, `internal/hostmem/`, `internal/idtable/`: the Go side of the host callbacks, the memory of a VM and the table of handles. `internal/measure/`: the tool that times builds, `go tool measure`, and `internal/measure/pages`, the program that renders tifa's pages for it. `internal/stress/`: the program that renders those pages for hours under random stops and checks that VMs stay correct and bounded (docs/internals/testing.md).
- `intl/`: `Intl` for a VM, in every locale Chromium has. `temporal/`: `Temporal` for a VM, computed by go-intl. `internal/gointl/`: our ECMA-402 and Temporal, which `intl/` and `temporal/` use, copied once from go-intl v0.6.1 and changed here freely since. Only go-intl's data is still taken: `internal/gointl/data.sh` makes the packs from the data of the release in `internal/gointl/UPSTREAM` (see docs/internals/upgrading.md). `internal/intlhook/`: what `temporal/` takes from `intl/`, and the checks every Builtin of both makes. `internal/vmhook/`: what a Go function of a package next to the engine asks of its VM (stop at the timeout, the room under the memory limit), see docs/internals/architecture.md. `url/`: `URL` and `URLSearchParams` for a VM, parsed in Go by `internal/whatwgurl` (a parser written after nlnwa/whatwg-url, with the IDNA code of x/net and x/text, and the web-platform-tests data it runs). `fmt/`: Go's fmt for a VM. `compare/`: a module of its own, with QuickJS's micro benchmarks. `compare/engines/`: another module, the comparison with the other engines that docs/use/benchmarks.md reports.
- `tifa/`: `tifa.Register`, which sets a VM up as tifa does (URL, Intl, Temporal and the functions named `__tifa_*`), for tifa, `cmd/tifajs` and `internal/measure/pages`.
- `cmd/tifajs/`: the `tifajs` command, which runs a JavaScript file in the engine, set up as tifa sets it up, to show a problem or run a script without Go ([docs/use/command.md](docs/use/command.md)). `.github/workflows/release.yml` and `scripts/tifajs-release.sh` build it for the six targets and attach it to the GitHub release of each tag.
- `assets/`: the logo, icon, banner and share card in `assets/brand/` (a popsicle with a lightning bolt drawn for liustack/tifajs after tifa's popsicle, and in the banner and the share card the gopher drawn after Renée French's Go gopher, CC BY 4.0, see NOTICE.md) and the WeChat QR code the READMEs show.
- [docs/internals/architecture.md](docs/internals/architecture.md) has the full map, the core types and the rules of the Go API.

## Docs (`docs/`)

1. Every doc starts with front matter: `summary` (one sentence saying what question the doc answers), `audience` (`user` or `dev`), `order` (a whole number, its place in reading order in its folder) and `read_when` (when to read it). Docs for people who use the module or the `tifajs` command (`audience: user`) are in `docs/use/`, and every other doc (`audience: dev`) is in `docs/internals/`. See the comment at the top of [scripts/docs-list.sh](scripts/docs-list.sh) for the format.
2. Before starting work and before creating a doc, run `scripts/docs-list.sh` to see the index, and read the docs whose Read when applies. Before committing, run `scripts/docs-list.sh --check`. It fails if a doc has no summary, audience or order, or is in the wrong folder.
3. What each doc is for:
   - [docs/internals/architecture.md](docs/internals/architecture.md): the layers, folders, core types and rules of the Go API
   - [docs/internals/changes/](docs/internals/changes/README.md): every change liustack/tifajs makes to the QuickJS C tree and to the Go API, by area
   - [docs/internals/testing.md](docs/internals/testing.md), [docs/internals/commit.md](docs/internals/commit.md): checks and commit rules
   - [docs/internals/allocator.md](docs/internals/allocator.md): the memory allocator of the C engine in `engine/`
   - [docs/internals/upgrading.md](docs/internals/upgrading.md): taking commits of upstream QuickJS and new intl data
   - [docs/internals/performance.md](docs/internals/performance.md): render speed of each release, how to measure it in tifa, and what was tried and not done
   - [docs/internals/measuring.md](docs/internals/measuring.md): how to time a change and judge it, the defaults and what they can see
   - docs/use/benchmarks.md: the comparison with the other JavaScript engines for Go and three native engines, among interpreters only (two tables: tifa pages and the V8 suite), run again only when the owner asks. It leaves out goja, sobek and otto. The files of the last run are kept in `compare/engines/results/`, how to run it is in `compare/engines/README.md`, and its changes are in `compare/engines/CHANGELOG.md`
   - [docs/internals/releasing.md](docs/internals/releasing.md): versions, tags, the release job and what follows a release
   - [docs/internals/upstream.md](docs/internals/upstream.md): where the code comes from and how QuickJS commits are taken in
   - [docs/use/api.md](docs/use/api.md): the Go API beyond Eval and Call, with the release of each function
   - [docs/use/command.md](docs/use/command.md): the `tifajs` command, its flags, limits and exit codes, and what it leaves out
   - [docs/use/from-modernc.md](docs/use/from-modernc.md): what a program moving from modernc.org/quickjs changes and runs into
   - [docs/use/index.md](docs/use/index.md) and [docs/internals/index.md](docs/internals/index.md): the page of each folder, with a `title`, `audience` and `order` and no summary
4. Each change liustack/tifajs makes to the QuickJS C tree or to the Go API gets one row in the index of its area under [docs/internals/changes](docs/internals/changes/README.md): what it does, its release and where it is described. The reasons, the numbers and the tests live in code comments and commit messages. Never write a change into README.md, which only links to the docs. New Go API goes into [docs/use/api.md](docs/use/api.md) with the release it came in. This is the rule for new writing: the write-ups already in `docs/internals/changes` stay as they are.
   - A claim about how this engine ranks among the other JavaScript engines for Go, in the README or elsewhere, needs a measurement in docs/use/benchmarks.md first.
5. [docs/internals/performance.md](docs/internals/performance.md) says how things are now: the table of what each release gained, how to measure in tifa, the list of what was tried and not done, and where to continue. The measurements of one change go in the commit message and in the doc of that change in docs/internals/changes, and detailed tables stay in the draft folder.
6. A changelog entry is one or two sentences.
7. Docs are written for readers: write conclusions and how to do things, not reasoning or work logs.
8. The docs for people who use the module or the `tifajs` command are docs/use/api.md, docs/use/command.md, docs/use/benchmarks.md, docs/use/from-modernc.md and the page of the folder, docs/use/index.md. Each has `audience: user` in its front matter, sits in `docs/use/`, and has a Chinese version next to it (`docs/use/api.zh-CN.md` and so on), and the two link to each other. Change both in the same commit: the Chinese never lags. Its front matter has `summary` and `read_when` in Chinese, the same `audience` and `order`, and `translatedFrom:` with the first 12 hex digits of the SHA-256 of the English file as it is now (the warning of the check prints it). Every other doc is English only. The root README.md and README.zh-CN.md link to each other, and a change to one changes the other in the same commit. `scripts/docs-list.sh --check` fails when a doc has no summary, audience or order, when a user doc is outside `docs/use/` or a dev doc is inside it, when a user doc has no Chinese version or its `translatedFrom` is not the hash of the English now, when a `.zh-CN.md` is next to no user doc, and when one of the two READMEs is missing.
9. English: keep it short and direct, use everyday words, and do not invent terms. Chinese (README.zh-CN.md and every `docs/use/*.zh-CN.md`): no semicolons, no dashes as punctuation, everyday words, and identifiers and product names kept in English. Numbers, versions and units are those of the English. Read every Chinese sentence with a negation or a condition again against the English one: translations here have turned a negation around before.
10. Code comments and error messages point to a doc by its path, such as `docs/internals/upgrading.md`, never to a section of the README.

## Issues and pull requests

liustack/tifajs does not accept pull requests: [CONTRIBUTING.md](CONTRIBUTING.md) says so, and `.github/PULL_REQUEST_TEMPLATE.md` tells whoever opens one. Bugs and ideas come in as issues through the templates in `.github/ISSUE_TEMPLATE/`, and security problems as private reports ([SECURITY.md](SECURITY.md)). Do not merge a pull request, and keep these files the same in spirit as those of liustack/modlens and tifa.

## License headers

- A new file written in liustack/tifajs starts with this header, in the comment syntax of its language and with the year it was created:

  ```go
  // Copyright (c) 2026 The liustack/tifajs Authors.
  // SPDX-License-Identifier: MIT
  // Use of this source code is governed by the MIT license in LICENSE at the
  // root of this repository.
  ```

- A file that came from upstream (QuickJS, go-intl), a generated file, and a new file made from one of them keep the upstream header and license, however much liustack/tifajs changes them. No code of modernc.org/quickjs remains: do not bring any back.
- Add a new author of liustack/tifajs to `AUTHORS`.
- New code or data from a third party keeps its license file next to it and gets a row in [NOTICE.md](NOTICE.md). Say MIT only together with what stays under the upstream licenses.

## Draft folder

Plans, design drafts, experiment notes and raw measurement data go in the repository draft folder `.issues/<YYYY-MM-DD-topic>/`, with the files for one task in one folder. `.issues/` is not checked in (add it to `.git/info/exclude` in a fresh clone), and checked-in code, docs and scripts must not refer to anything in it. Conclusions that are settled and need to be kept long term are cleaned up and written into `docs/`.

## Verification and commits

- See [docs/internals/testing.md](docs/internals/testing.md) for verification commands and [docs/internals/commit.md](docs/internals/commit.md) for commit rules. Every commit passes `go build ./...`, `go vet ./...` and `scripts/docs-list.sh --check`, and `go test -count=1 -p 1 .` (about a minute). The branch passes `make check-full` before it is merged.
- The generated files are large. Build and test a few packages at a time (`-p 2`), and run heavy jobs such as test262, `make build_all_targets` or the race detector one after another, not in parallel.
- For new behavior, write a failing test first, then implement. For a bug, find the root cause before changing code, and do not hide it with fallback values.
- After a new release of QuickJS, follow [docs/internals/upgrading.md](docs/internals/upgrading.md).
- Tools are Go programs or short shell scripts. Do not add Python.
- When merging a branch that changes what programs using the module can see, add it to CHANGELOG.md.

## Releases

See [docs/internals/releasing.md](docs/internals/releasing.md).

- Release patch versions only (v0.26.17, v0.26.18, ...). Do not move to a new minor version for a small problem.
- A release is a lightweight tag on the merge commit of `main`: that is the version of the module. Pushing it runs the release job, which makes the GitHub release of the tag with the binaries of `tifajs` and nothing else (docs/internals/releasing.md). Check that the job passed.
- Ask the user before you create a tag, and again before you push one. Never push `main` or a tag on your own.
- The comparison with the other engines runs only when the owner asks for it: it serves how the project is presented, not the work on speed, and no release waits for it. When it runs, it runs on the final engine of a release and updates docs/use/benchmarks.md and the README tables as docs/internals/releasing.md says. After every release, upgrade tifa to it and measure the release on tifa's pages.

