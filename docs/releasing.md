---
summary: 'How a release is made: patch versions only, a lightweight tag on the merge commit of main, asking before tagging and pushing, what to update before, and after it running the comparison with other Go engines again, upgrading tifa and collecting its engine profile again'
read_when:
  - A branch was merged into main and is ready to ship
  - Choosing the version of the next release
  - About to create or push a tag
  - A release is out and tifa should use it
  - Running the comparison with other JavaScript engines for Go again
---

# Releasing

## Versions

- Releases are patch versions: v0.26.0, v0.26.1, and so on. A fix or a speedup, even a large one, is the next patch version. Do not move to a new minor version for a small problem: that is for the owner of the repository to decide.
- `github.com/liustack/quickjs` has no GitHub releases. A release is a tag, and users get it with `go get github.com/liustack/quickjs@<version>`.

## Before the release

On the branch, before it is merged:

1. The checks of [testing.md](testing.md#before-committing) pass, and those for what the branch touches (the focused checks, test262, `make build_all_targets`).
2. Each change is in the doc of its area under [fork-changes](fork-changes/README.md), with its row in the index, and a change for speed is in [performance.md](performance.md).
3. [CHANGELOG.md](../CHANGELOG.md) and [CHANGELOG.zh-CN.md](../CHANGELOG.zh-CN.md) say what the branch changes for users. The last commit of the branch puts it under the new version and the date (`docs(changelog): release v0.26.17`), so that the merge commit you tag carries it.

Then merge it into `main` with a merge commit, `merge: <what the branch does>` (see [commit.md](commit.md#branches-and-merges)).

## Tagging

Ask the owner before you create a tag and again before you push one. A pushed tag is public at once and the Go module proxy keeps it for good: it cannot be taken back, only replaced by the next version.

1. Tag the merge commit of `main` with a lightweight tag, as every release so far:

   ```sh
   git log --first-parent --oneline -3 main    # the merge commit to tag
   git tag v0.26.17 <merge commit>
   git cat-file -t v0.26.17                    # commit, not tag
   ```

2. Push `main` and the tag:

   ```sh
   git push origin main
   git push origin v0.26.17
   ```

3. Check that the proxy has it: `GOPROXY=https://proxy.golang.org go list -m github.com/liustack/quickjs@v0.26.17`.

## After the release

### Compare with the other engines again

Every release runs the comparison with the other pure Go JavaScript engines again ([benchmarks.md](benchmarks.md)). It lives in `compare/engines`, a module of its own, and runs on a Linux machine with a CPU and its hyperthread sibling kept free for it (CPU 1 and CPU 5 below):

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
   tifa/run.sh "$page_dir" bin "$HOME/jsnative"
   python3 behind.py "results/$run.json"
   ```

   With `-cpu 1 -sibling 5` the driver pins timed processes to CPU 1 with GOMAXPROCS=1. `tifa/run.sh` pins both functional checks (normal, stub and proxy) and timed processes to CPU 1, with GOMAXPROCS=1 and TZ=UTC. Both stages use the same 4 GiB capped entry and allow at most three tries for CPU disturbance. The binaries are built with `-pgo=off`. On a shared machine, cut the run into pieces with `-add` and run test262 on another CPU at low priority, as [benchmarks.md](benchmarks.md#rerun) shows. When an engine fails shop pages and closes its runtime, measure the other pages in a run of their own, as the same section says.

3. After any rest runs or single-page probes, write and commit both page files with one command:

   ```sh
   python3 tifa/pages.py "$page_dir" --output "results/$run-pages" &&
     git add "results/$run-pages.md" "results/$run-pages.json" &&
     git commit -m "docs(bench): record $run pages" -m "Archive page times and normalized HTML digests so equality can be checked from the saved results."
   ```

   The JSON keeps raw process results, rest runs, probes, tries and per-page HTML comparisons. Commit `results/<run>.json` and `results/<run>.md` too, update the tables of [benchmarks.md](benchmarks.md) and the list of where the engine is far behind, and add the run to its History.
4. If the method changed (a workload, an engine, a pinned input, a setting), add an entry to `compare/engines/CHANGELOG.md`.

### Upgrade tifa

The fork exists for tifa, so every release goes into tifa next:

1. In tifa, `go get github.com/liustack/quickjs@v0.26.17` and `go mod tidy`.
2. Collect tifa's engine profile again, as tifa's [docs/testing.md](https://github.com/liustack/tifa/blob/main/docs/testing.md#the-engine-profile) says under "The engine profile": PGO finds call sites by line, so a profile of the old engine matches fewer of them. `TestEngineProfileMatchesTheDependencies` in tifa's `internal/codegen` fails until it is collected again.
3. Measure the release on tifa's pages ([Measuring in tifa](performance.md#measuring-in-tifa)), with one VM and with several, and put the numbers into tifa's docs/performance.md and its changelogs.
4. Run tifa's checks. The SSR soak tests (`TIFA_SSR_SOAK=1`) also cover running out of memory and VM reuse.
