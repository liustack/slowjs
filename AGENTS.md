# quickjs (project notes for AI agents)

## Goal

`github.com/liustack/quickjs` is a pure Go QuickJS kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages to HTML inside the Go process. It is a fork of `modernc.org/quickjs` (the Go API, at the root) and `modernc.org/libquickjs` (QuickJS translated to Go by ccgo, under `lib/`), merged into one module. The fork makes renders faster, makes limits hold (a timeout or running out of memory ends the call whatever the script does), and makes output the same on every machine, while staying close enough to upstream to take its releases.

## Hard constraints

- Pure Go, `CGO_ENABLED=0`. No cgo, no C compiler, no shared library, on all 14 targets. Do not bring in a cgo engine.
- The generated engine (`lib/ccgo_*.go`, `lib/internal/run-test262/ccgo_*.go`) is never edited by hand. A change to the engine is one of two kinds:
  - a mechanical rewrite of the generated Go by `internal/fixlib`, which runs again after every upstream release
  - a change in C, kept in `lib/internal/patch/quickjs.diff` so that a regeneration of `lib/` keeps it, with its Go form put in by `fixlib`
- Every change of `fixlib` checks the code it touches against a fingerprint or a count, and stops with an error when upstream changes that code, so that it never rewrites code it has not seen. A change that `fixlib` undoes before the other changes and makes again after them must check that undoing it gives back the text it was given, byte for byte.
- The committed generated files must be exactly what `fixlib` makes from the upstream files, byte for byte (see [docs/testing.md](docs/testing.md#same-as-upstream-after-fixlib)).
- A change for speed leaves what the engine gives the same: the same HTML of tifa's pages, the same 58 known test262 errors, and the same TLS stack slots at every stack check, so a script recurses exactly as deep as before.

## Current state of the repository

- `main` carries the fork. `master` and the other upstream branches mirror gitlab.com/cznic/quickjs and get no commits of their own. Releases v0.26.0 to v0.26.16 are lightweight tags on merge commits of `main`.
- Root: the Go API (`quickjs.go`, `quickjs32.go`, `quickjs64.go`, `promise.go`, `tlsstack_*.go`) and its tests.
- `lib/`: the engine, merged from upstream libquickjs with `git subtree`, plus the fork's hand-written Go next to the generated files. `lib/README.md`, `lib/CLAUDE.md`, `lib/Makefile` and `lib/generator.go` are upstream's and kept as they are so that subtree merges stay clean: they describe libquickjs as a module of its own. Where they disagree with this file, this file wins. This fork does not regenerate `lib/` with `make generate`.
- `internal/fixlib/`: the command that puts the fork's changes into the generated Go.
- `intl/`: `Intl` for a VM, in every locale Chromium has. `fmt/`: Go's fmt for a VM. `compare/`: a module of its own that benchmarks against other engines.
- [docs/architecture.md](docs/architecture.md) has the full map, the core types and the rules of the Go API.

## Docs (`docs/`)

1. Every doc starts with front matter: `summary` (one sentence saying what question the doc answers) and `read_when` (when to read it). See the comment at the top of [scripts/docs-list.sh](scripts/docs-list.sh) for the format.
2. Before starting work and before creating a doc, run `scripts/docs-list.sh` to see the index, and read the docs whose Read when applies. Before committing, run `scripts/docs-list.sh --check`. It fails if a doc has no summary.
3. What each doc is for:
   - [docs/architecture.md](docs/architecture.md): the layers, folders, core types and rules of the Go API
   - [docs/fork-changes/](docs/fork-changes/README.md): every change the fork makes to upstream, by area
   - [docs/testing.md](docs/testing.md), [docs/commit.md](docs/commit.md): checks and commit rules
   - [docs/upgrading.md](docs/upgrading.md): taking upstream releases and new intl data
   - [docs/performance.md](docs/performance.md): render speed of each release, measured in tifa
   - [docs/benchmarks.md](docs/benchmarks.md): the comparison with other JavaScript engines for Go, with its history, run again at every release. Its results are kept under `compare/results/` and its changes in `compare/CHANGELOG.md`
   - [docs/releasing.md](docs/releasing.md): versions, tags and what follows a release
   - [docs/upstream.md](docs/upstream.md): the upstream README
4. Each change the fork makes to upstream is written up in the doc of its area under [docs/fork-changes](docs/fork-changes/README.md), with a row in its index: what it does, why it is safe, what it gains or costs and how that was measured, its tests, and what to do when `fixlib` stops on it. A change for speed also goes into [docs/performance.md](docs/performance.md). The comparison with other JavaScript engines for Go is [docs/benchmarks.md](docs/benchmarks.md): a claim about how this engine ranks among them, in the README or elsewhere, needs a measurement there first.
5. Docs are written for readers: write conclusions and how to do things, not reasoning or work logs.
6. Docs are written in English. README.md and CHANGELOG.md also have a `*.zh-CN.md` Chinese version: when you change one side, update the other to match. `scripts/docs-list.sh` skips `*.zh-CN.md` files.
7. English: keep it short and direct, use everyday words, and do not invent terms. Chinese: do not use semicolons, and do not use dashes as punctuation.
8. Code comments and error messages point to a doc by its path, such as `docs/upgrading.md`, never to a section of the README.

## Draft folder

Plans, design drafts, experiment notes and raw measurement data go in the repository draft folder `.issues/<YYYY-MM-DD-topic>/`, with the files for one task in one folder. `.issues/` is not checked in (add it to `.git/info/exclude` in a fresh clone), and checked-in code, docs and scripts must not refer to anything in it. Conclusions that are settled and need to be kept long term are cleaned up and written into `docs/`.

## Verification and commits

- See [docs/testing.md](docs/testing.md) for verification commands and [docs/commit.md](docs/commit.md) for commit rules. Every commit passes `go build ./...`, `go vet ./internal/fixlib`, `go run ./internal/fixlib -check` and `scripts/docs-list.sh --check`, and the branch passes `go test -short -p 2 -timeout 30m . ./lib ./internal/fixlib` before it is merged (the tests of `internal/fixlib` take 8 to 13 minutes).
- The generated files are large. Build and test a few packages at a time (`-p 2`), and run heavy jobs such as test262, `make build_all_targets` or the race detector one after another, not in parallel.
- For new behavior, write a failing test first, then implement. For a bug, find the root cause before changing code, and do not hide it with fallback values.
- After an upstream release, follow [docs/upgrading.md](docs/upgrading.md). When `fixlib` stops, read the code it names before updating any list.
- When merging a branch that changes what programs using the module can see, add it to both CHANGELOG.md and CHANGELOG.zh-CN.md.

## Releases

See [docs/releasing.md](docs/releasing.md).

- Release patch versions only (v0.26.17, v0.26.18, ...). Do not move to a new minor version for a small problem.
- A release is a lightweight tag on the merge commit of `main`. There are no GitHub releases.
- Ask the user before you create a tag, and again before you push one. Never push `main` or a tag on your own.
- After a release, run the comparison with the other engines again and update [docs/benchmarks.md](docs/benchmarks.md), then upgrade tifa to it, collect tifa's engine profile again as tifa's docs/testing.md says ("The engine profile"), and measure the release on tifa's pages.
