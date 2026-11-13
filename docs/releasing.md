---
summary: 'How a release is made: patch versions only, a lightweight tag on the merge commit of main, asking before tagging and pushing, what to update before, and after it running the comparison with other Go engines again and upgrading tifa'
read_when:
  - A branch was merged into main and is ready to ship
  - Choosing the version of the next release
  - About to create or push a tag
  - A release is out and tifa should use it
  - Running the comparison with other JavaScript engines for Go again
---

# Releasing

English · [简体中文](releasing.zh-CN.md)

## Versions

- Releases are patch versions: v0.30.0, v0.30.1, and so on. A fix or a speedup, even a large one, is the next patch version. A release that removes or changes a public API is the next minor version, as v0.27.0 was for the module loaders it removed. Do not move to a new minor version for anything else: that is for the owner of the repository to decide.
- `github.com/liustack/quickjs` has no GitHub releases. A release is a tag, and users get it with `go get github.com/liustack/quickjs@<version>`.
- The versions from v0.28.0 to v0.29.11 are at `github.com/liustack/slowjs`, all others at `github.com/liustack/quickjs`. The Go module proxy keeps serving the old ones there, and the `go.mod` of v0.29.11 marks the module deprecated and names the new path. How the rename went is in [upstream.md](upstream.md#moving-from-githubcomliustackslowjs).

## Before the release

On the branch, before it is merged:

1. The checks of [testing.md](testing.md#before-committing) pass, and those for what the branch touches (the focused checks, test262, `make build_all_targets`). `make check-full` passes, on a machine with LLVM 22.1.8: the checked-in engine is what the generator makes, and the root package, the race detector, the sanitizers and test262 pass. `scripts/go-versions.sh` passes: the checks of the engine with every patch release of Go the module lets build, from the lowest Go of `go.mod` up (the Go tests, and on a machine with LLVM the generator tests and the post-link check, see [testing.md](testing.md#ci)).
2. Each change is in the doc of its area under [changes](changes/README.md), with its row in the index, and a change for speed is in [performance.md](performance.md).
3. [CHANGELOG.md](../CHANGELOG.md) and [CHANGELOG.zh-CN.md](../CHANGELOG.zh-CN.md) say what the branch changes for users. The last commit of the branch puts it under the new version and the date (`docs(changelog): release <version>`), so that the merge commit you tag carries it.
4. In [changes](changes/README.md), the changes of this release still say "Unreleased". Replace it with the new version in the tables of the index and in the `Release:` line of each area doc (`grep -rn Unreleased docs/changes`, leaving the sentence under "Adding a change"), in the same last commit as the changelogs. Nothing else does it, and a row left at "Unreleased" after the tag is wrong from then on.
5. The profile is not stale where it matters: see [The profile](#the-profile) below.

Then merge it into `main` with a merge commit, `merge: <what the branch does>` (see [commit.md](commit.md#branches-and-merges)).

## The profile

The engine is generated with a profile of page renders (`engine/pgo/profile.txt`, see [engine/pgo/README.md](../engine/pgo/README.md)). A function whose C changed since the training has no usable profile and is compiled as it would be without one. The rest of the engine keeps its gain. So an old profile only loses part of the gain, and nothing else breaks.

`make check-full` prints the list: the lines of the generator that start with `warning: profile:`, `profile:` and `profile `, for each architecture, in the output of the `gen` step. `scripts/check-gen.sh` prints the same lines by itself. Read them before a release:

- If `JS_RegvmRun` is on the list, train again. It is the register interpreter, and without its profile only 60% to 80% of the gain is left (measured on tifa's pages).
- If the generator says the C of more than 30 functions changed, train again. The profile is old.
- A short list of other functions needs nothing. Train again when you want the gain back for them.

To train again, on a machine with LLVM 22.1.8:

```sh
LLVM_BIN=/usr/lib/llvm-22/bin engine/pgo/train.sh
make gen && make gen ARCH=arm64
git add engine/pgo/profile.txt internal/cengine
make check-gen
git commit -m "build(pgo): train the profile again"
```

The new profile and the files it changes go in one commit. Then run `make check-gen` again: no `warning: profile:` line should be left (the `profile:` line about `likely()` is a note, not a warning). Train again also after taking commits of upstream QuickJS ([upgrading.md](upgrading.md)) and after moving to another LLVM, whose profile format differs.

## Tagging

Ask the owner before you create a tag and again before you push one. A pushed tag is public at once and the Go module proxy keeps it for good: it cannot be taken back, only replaced by the next version.

1. Tag the merge commit of `main` with a lightweight tag, as every release so far:

   ```sh
   git log --first-parent --oneline -3 main    # the merge commit to tag
   git tag <version> <merge commit>
   git cat-file -t <version>                    # commit, not tag
   ```

2. Push `main` and the tag:

   ```sh
   git push origin main
   git push origin <version>
   ```

3. Check that the proxy has it: `GOPROXY=https://proxy.golang.org go list -m github.com/liustack/quickjs@<version>`.

## After the release

### Compare with the other engines again

A minor release runs the comparison with the other pure Go JavaScript engines again ([benchmarks.md](benchmarks.md)). Patch releases skip it unless the owner asks: while the engine changes from week to week, a new table each patch costs hours of a machine and says little. It lives in `compare/engines`, a module of its own, and runs on a Linux machine with a CPU and its hyperthread sibling kept free for it (CPU 1 and CPU 5 below):

1. Point the comparison at the release, and update the other engines if their newest versions should be in the run:

   ```sh
   cd compare/engines
   go get <module>@latest && go mod tidy      # optional, for each other engine
   ```

   `compare/engines` reaches this module through a `replace` of the repository root, so the run measures the checkout it is in: check out the tag first.

2. Run every workload, five rounds, and the tifa pages with the reference rows:

   ```sh
   run="<date>-<version>-<host>"
   go run ./cmd/compare -cpu 1 -sibling 5 -name "$run"
   tifa/polyfill/build.sh && native/build.sh "$HOME/jsnative"
   page_dir=$(mktemp -d "/tmp/$run-pages.XXXXXX")
   tifa/run.sh "$page_dir" bin "$HOME/jsnative"    # TIFA_ONLY=a,b runs only some engines
   go run ./cmd/behind "results/$run.json"
   ```

   With `-cpu 1 -sibling 5` the driver pins timed processes to CPU 1 with GOMAXPROCS=1. `tifa/run.sh` pins both functional checks (normal, stub and proxy) and timed processes to CPU 1, with GOMAXPROCS=1 and TZ=UTC. Both stages use the same 4 GiB capped entry and allow at most three tries for CPU disturbance. The binaries are built with `-pgo=off`. On a shared machine, cut the run into pieces with `-add` and run test262 on another CPU at low priority, as [benchmarks.md](benchmarks.md#rerun) shows. When an engine fails shop pages and closes its runtime, measure the other pages in a run of their own, as the same section says.

3. After any rest runs or single-page probes, write and commit both page files with one command:

   ```sh
   go run ./cmd/pages "$page_dir" --output "results/$run-pages" &&
     git add "results/$run-pages.md" "results/$run-pages.json" &&
     git commit -m "docs(bench): record $run pages" -m "Archive page times and normalized HTML digests so equality can be checked from the saved results."
   ```

   The JSON keeps raw process results, rest runs, probes, tries and per-page HTML comparisons. Commit `results/<run>.json` and `results/<run>.md` too, update the tables of [benchmarks.md](benchmarks.md) and the list of where the engine is far behind, and add the run to its History.
4. If the method changed (a workload, an engine, a pinned input, a setting), add an entry to `compare/engines/CHANGELOG.md`.

### Upgrade tifa

liustack/quickjs exists for tifa, so every release goes into tifa next:

1. In tifa, `go get github.com/liustack/quickjs@<version>` and `go mod tidy`.
2. Put the numbers each change was measured with before it merged into tifa's docs/performance.md and its changelogs. Do not measure the release again as a whole: each change was already measured against main. Measure on tifa's pages ([Measuring in tifa](performance.md#measuring-in-tifa)) only when the owner asks, or when two changes of the release may affect each other.
3. Run tifa's checks. The SSR soak tests (`TIFA_SSR_SOAK=1`) also cover running out of memory and VM reuse.
