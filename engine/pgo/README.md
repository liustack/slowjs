# engine/pgo

What `make gen` needs to compile the engine with a profile: the profile itself, the program that trains it, and the pages it trains on. What the profile does to the generated code, what it gains and where it stops is in [docs/changes/generator.md](../../docs/changes/generator.md#a-profile-of-tifas-pages).

## What is here

| Path | What |
| --- | --- |
| `train.sh` | Trains the profile. Builds the engine as the generator compiles it with counters in, renders the pages, runs `loops.js`, checks it and writes `profile.txt` |
| `profile.txt` | The profile, in the text form of `llvm-profdata`. One file for amd64 and arm64: the hash of a function depends on the C only. Training twice gives files that differ in a few counts, so the file is checked in and `make gen` never trains |
| `pages/` | The pages the training renders (below) |
| `loops.js` | The second input: plain loops over paths the pages never run (below) |
| `cover.awk` | The check of `loops.js` that `train.sh` runs (below) |

## Train again

On Linux or macOS with LLVM 22.1.8 (`scripts/llvm-bin.sh` finds it, or `LLVM_BIN` names its bin folder):

```sh
engine/pgo/train.sh   # about a minute
make gen && make gen ARCH=arm64
git add engine/pgo/profile.txt internal/cengine
```

`train.sh` stops when it finds no LLVM 22.1.8. It works in `.build/pgo-train` (`TRAIN_OUT` changes it). It needs nothing outside the repository. The profile and the generated files go in one commit, as `make check-gen` wants. When to train again is in [docs/releasing.md](../../docs/releasing.md#the-profile).

## The loops

The pages leave many paths of the engine at a count of 0: typed arrays, DataView, regular expressions on text that is not Latin-1, BigInt, `Date.parse`, the slow paths of the operators, functions that pass their arguments on with `apply` and more. LLVM takes a count of 0 for a path that never runs, moves spills into it and its blocks out of the way, and a script that runs it pays for that. `loops.js` runs each such kind of path a few thousand times. It is merged with the pages as it is, neither input scaled: its counts are about 3% of the profile, enough to lift those paths above 0 and far too few to change what the pages run. What it gains and costs is in [docs/changes/generator.md](../../docs/changes/generator.md#a-second-input-plain-loops).

The loops are written for the training. The micro benchmarks of `compare/testdata` measure what the profile does to paths it was not trained on, so none of them may go into `loops.js`, not even in part.

`train.sh` checks the loops (`cover.awk`) and stops when the check fails:

- It runs the 64 micro benchmarks (N=100, for the check only: their counts never go into the profile) and takes the paths the pages leave cold and the benchmarks run at least 100 times. It prints how many of them `loops.js` runs, `engine/pgo/loops.js: runs 1553 of the 1601 paths ... (97.0%, at least 95%)`, and stops below 95% (`COVER_FLOOR`).
- Each part of `loops.js` is there for a function, listed in `need` in `train.sh`, and the training stops when the loops no longer run one of them, or when it no longer exists.

When the C changes, the check can fail. Then write or change a loop for the paths it names, and train again. Lower the floor only with the reason in the commit.

## The pages

The training renders real pages of two apps of [tifa](https://github.com/liustack/tifa), on the native test host (`engine/test/render.c`, `tifa=1`: it defines the four functions tifa gives each VM, as tifa does). Each page list is rendered twice and the counters are set to zero when the second pass starts, so the profile holds warm renders only: no load of the bundle and no first render.

| Path | What | Size |
| --- | --- | --- |
| `pages/shop/shop.js` | The server bundle of tifa's `examples/shop`, built as an ES module and turned into a script by `compare/engines/cmd/modulescript`. React, tifa's runtime and the packages the app uses | 1.6 MB |
| `pages/shop/in/*.json` | The input of 20 pages of the shop: 6 of the workbench (`wb-studio`, `wb-order-studio`, `wb-stats-studio` and the same of `weekend`) and 14 of the store in en-US and zh-CN (home, products, product, journal, post, cart, checkout). The training renders the 3 of `studio` and the 7 in en-US, all of them are timed (`internal/measure/pages`) | 0.35 MB |
| `pages/mui/mui.js` | The server bundle of tifa's `test/apps/mui`, made the same way. React, MUI and emotion | 0.76 MB |
| `pages/mui/in/*.json` | The input of 3 pages of that app: `home`, which the training renders, `orders` and `order` | 24 KB |
| `pages/fakeintl.js` | A small `Intl` the native host needs to load the bundles, because the C engine has none (it comes from the Go packages `intl` and `temporal`). It formats numbers with `toFixed`, dates as ISO dates and plurals as one or other, so the HTML differs from tifa's real pages in those places, and the work done inside Intl is not in the profile |
| `pages/THIRD_PARTY_NOTICES.md` | The packages inside the two bundles with their versions, licenses and license texts |

An input is the JSON text tifa hands to `startRender` for a page: `buildId`, `language`, `alternates`, `url`, `data`, `preload`, `session` and the rest, with the texts of the loaders put into the result as `json`, and no `nonce`.

### Make them again

The bundles and inputs came from tifa at commit `07b6ce0f7e052558d47b8ff649623bebd047ae04`, with the packages of each app's `pnpm-lock.yaml`, Go 1.27.2, Node 24.13.0 and pnpm 10.33.0. The files here are the reference: the training reads them and the SHA-256 below names them. The steps below are how they were made. Both bundles were built again with the packages of the lockfiles, taken from an existing install, and gave the same bytes. A fresh `pnpm install --frozen-lockfile` has not been run.

```sh
# 1. a clean copy of tifa at the commit, with the packages installed
git clone https://github.com/liustack/tifa && cd tifa && git checkout 07b6ce0f7e052558d47b8ff649623bebd047ae04
(cd examples/shop && pnpm install --frozen-lockfile)
(cd test/apps/mui && pnpm install --frozen-lockfile)

# 2. the bundles: <app>/.tifa/dist/ssr.js, then the script form
(cd examples/shop && go tool tifa build)
(cd test/apps/mui && go tool tifa build)
(cd /path/to/slowjs/compare/engines &&
  go run ./cmd/modulescript /path/to/tifa/examples/shop/.tifa/dist/ssr.js /path/to/slowjs/engine/pgo/pages/shop/shop.js &&
  go run ./cmd/modulescript /path/to/tifa/test/apps/mui/.tifa/dist/ssr.js /path/to/slowjs/engine/pgo/pages/mui/mui.js)

# 3. the notices, which read the source maps of the build
(cd /path/to/slowjs/compare/engines && go run ./cmd/notices /path/to/tifa ../../engine/pgo/pages)
```

The SHA-256 of `shop.js` is `0d2f9542f12c68959572a2dc3e8a0775a01244cfcc0f59778413e6911d3ea6a3` and that of `mui.js` is `3c8d8c50d448f80b67e92f89228043c5b8cf7cae7f0268254c593e56f5dbcd1c`. The notices tool stops when a bundle is not the one in `pages/`.

To record the inputs, put this file in tifa's `internal/ssr/` as `zz_dump.go` and add `dumpInput(input, in.Texts)` after the input is encoded in `internal/ssr/pool.go` (`renderInVM`, right after the `encode input` error check):

```go
package ssr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// dumpInput writes the input of a render, with each text put into its result as `json`
// and without the nonce, so that a host that passes only the input to startRender can render it.
func dumpInput(input []byte, texts []string) {
	dir := os.Getenv("TIFA_DUMP_INPUT")
	if dir == "" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		panic(err)
	}
	delete(m, "nonce")
	data, _ := m["data"].(map[string]any)
	for _, v := range data {
		r := v.(map[string]any)
		if t, ok := r["text"]; ok {
			r["json"] = texts[int(t.(float64))]
			delete(r, "text")
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	name := strings.NewReplacer("/", "_", "?", "@", "&", "+", "=", "-").Replace(m["url"].(string))
	if err := os.WriteFile(filepath.Join(dir, name+".json"), out, 0o644); err != nil {
		panic(err)
	}
}
```

Rebuild each app, start it with `--ssr-memo-size 0` (so each request renders) and, for the shop, `SHOP_OPEN=1` (lets the workbench in without signing in), with `TIFA_DUMP_INPUT` set to an empty folder, and ask for each page once with no cookies. The shop's pages are `/projects/studio`, `/projects/studio/orders/SH-1042`, `/projects/studio/stats` and `/store`, `/store/products`, `/store/products/pour-over-kettle`, `/store/journal`, `/store/journal/stock-sync-api`, `/store/cart`, `/store/checkout`. MUI's home page is `/`. Each file in the folder is named after the URL: rename it to the name in `pages/<app>/in/`.
