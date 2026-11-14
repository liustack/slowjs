---
summary: 'How a release is made: patch versions only, a lightweight tag on the merge commit of main, asking before tagging and pushing, what to update before, the release job that builds the slowjs command and attaches it to the GitHub release of the tag, and after it running the comparison with other Go engines again and upgrading tifa'
read_when:
  - A branch was merged into main and is ready to ship
  - Choosing the version of the next release
  - About to create or push a tag
  - The release job failed, or a GitHub release lacks the binaries of slowjs
  - A release is out and tifa should use it
  - Running the comparison with other JavaScript engines for Go again
---

# Releasing

English · [简体中文](releasing.zh-CN.md)

## Versions

- Releases are patch versions: v0.30.0, v0.30.1, and so on. A fix or a speedup, even a large one, is the next patch version. A release that removes or changes a public API is the next minor version, as v0.27.0 was for the module loaders it removed. Do not move to a new minor version for anything else: that is for the owner of the repository to decide.
- A version of the module is a tag, and users get it with `go get github.com/liustack/quickjs@<version>`. From v0.31.0, pushing the tag also makes a GitHub release of it, which carries only the prebuilt binaries of the `slowjs` command ([command.md](command.md)): see [The release job](#the-release-job).
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
engine/pgo/train.sh
make gen && make gen ARCH=arm64
git add engine/pgo/profile.txt internal/cengine
make check-gen
git commit -m "build(pgo): train the profile again"
```

`train.sh` stops when its check of `engine/pgo/loops.js` fails: the loops no longer run enough of the paths the pages leave cold, or a part of them no longer reaches its function. [engine/pgo/README.md](../engine/pgo/README.md#the-loops) says what to do. Do not put the micro benchmarks of `compare/testdata` into the loops.

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
4. Check that the release job made the GitHub release of the tag (`gh run list --workflow release.yml`, then the release page): five archives and `checksums.txt`.

### The release job

`.github/workflows/release.yml` runs when a tag `v*` is pushed. The GitHub release it makes is there only for the binaries of `slowjs`: the module is still the tag, and Go users never need the release.

1. **build** (linux/amd64): `scripts/slowjs-release.sh <tag> dist` builds `cmd/slowjs` for the five targets with `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"` and the tag stamped in as the version, with the Go of the `go` line of `go.mod`, as CI tests with. It packs each binary with its license texts and writes `checksums.txt` (SHA-256).
2. **test**, on each of the five systems as CI runs them (darwin/amd64 under Rosetta): it checks the archive against `checksums.txt`, unpacks it, and runs the tests of `cmd/slowjs` against that binary (`SLOWJS_BIN`, see [command.md](command.md#tests)), the V8 suite among them, with the tag as the version it must report.
3. **release**, only after all five pass and only for a pushed tag: it makes the GitHub release of the tag, titled with the tag, with the archives and `checksums.txt`, and notes that link to [command.md](command.md) and the changelog. It is the only job with `contents: write`. Run again for a tag whose release exists, it replaces the files.

What a release holds:

| File | What it is |
| --- | --- |
| `slowjs-x86_64-unknown-linux-gnu.tar.gz` | linux/amd64 |
| `slowjs-aarch64-unknown-linux-gnu.tar.gz` | linux/arm64 |
| `slowjs-x86_64-apple-darwin.tar.gz` | darwin/amd64 |
| `slowjs-aarch64-apple-darwin.tar.gz` | darwin/arm64 |
| `slowjs-x86_64-pc-windows-msvc.zip` | windows/amd64 |
| `checksums.txt` | The SHA-256 of each archive, as `sha256sum` writes it |

The names carry the target as Rust names it, and no version, so that installers of prebuilt binaries find them and `releases/latest/download/<name>` always points to the newest. Each archive holds one folder, `slowjs-<target>`, with the binary (`slowjs`, or `slowjs.exe`), `NOTICE.md` and the texts [NOTICE.md](../NOTICE.md#what-a-program-that-ships-a-binary-must-include) says a binary must carry, at their paths in this repository: `LICENSE`, `LICENSE-MIT`, `LICENSE-LIBQUICKJS`, `LICENSE-MODERNC-LIBQUICKJS`, `LICENSE-TLSF`, `engine/libc/COPYRIGHT`, and under `third_party/` the license of Go and of golang.org/x/sys. The command links no Intl, so the notices of `intl` and `internal/gointl` are not needed. The script stops when the binary links a module other than golang.org/x/sys, which would ship without its license.

Before the branch of a release is merged, run the job by hand on it (`gh workflow run release.yml --ref <branch>`): it builds and tests the binaries, with `dev-<commit>` as the version, and makes no release. The same build runs locally with `scripts/slowjs-release.sh <version> <dir>`.

When the job fails after the tag is pushed, the module is out all the same: the tag stays. A failure of the machine is fixed by running the failed jobs again. A failure that needs a change of the code ships its fix in the next patch version, whose release then has the binaries.

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
