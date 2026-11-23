---
summary: 'Where the engine and the Go API come from, how a commit of upstream QuickJS is taken in, and who wrote the code liustack/tifajs is built on'
audience: dev
order: 7
read_when:
  - Asking which QuickJS commit the engine is built on, or who wrote the code
  - Taking a commit of upstream QuickJS into the engine
---

# Upstream

## Where the code comes from

- **The engine** is [QuickJS](https://bellard.org/quickjs), the JavaScript engine in C by Fabrice Bellard and Charlie Gordon, at the release of 2026-06-04, commit `3d5e064e9dd67c70f7962836505a7fa067bf0a4e`. `engine/` holds that C with the changes of liustack/tifajs on top. `internal/goasm/gen` compiles it to Go assembly, which is checked in. The generated files are never edited by hand, and `make check-gen` fails when they are not what the generator makes. The test262 suite gives 33 known failures, listed in `engine/test/test262_errors.txt`.
- **The Go API** (`VM`, `Value`, `Eval`, `Call` and the rest at the root of the module) started from [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) v0.25.0 (2026-10-01), commit `ea7b851` in this repository's history. Its code has since been replaced in full by code written for liustack/tifajs, so none of it remains. Until v0.28 the engine under it was [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs), QuickJS translated to Go by ccgo. That engine is gone, and so is its Go. What a program moving from modernc.org/quickjs runs into is in [from-modernc.md](../use/from-modernc.md).
- **The Go side of `Intl` and `Temporal`** (`internal/gointl`) was copied from [go-intl](https://github.com/go-quickjs/go-intl) v0.6.1, commit `59e7b7a9f80bb90458f314e7d99efa4fb592dc3b`, once, and is changed here freely since: no later go-intl code is taken. Only its data is: the packs are made from the `data/` directory of the go-intl release in `internal/gointl/UPSTREAM`, cut to the locales of Chromium, with the time zones of `internal/gointl/qjstz` (see [upgrading.md](upgrading.md#updating-the-data-of-intl-internalgointl)).
- **Everything else** is written for liustack/tifajs, or comes from a project named in [NOTICE.md](../../NOTICE.md): the allocator (TLSF), the small libc of the engine (musl), the locale data and the Go side of `Intl` and `Temporal` (go-intl, ICU, Chromium).

The authors of liustack/tifajs are in [AUTHORS](../../AUTHORS).

## Taking QuickJS commits

The C tree in `engine/` does not follow upstream release by release. A commit of upstream QuickJS made after the pinned one is taken into it on its own, with a test, when it fixes a wrong answer, a leak or a crash (whether or not a page can reach it) or makes renders faster, and a new feature or a change for speed when tifa has a use for it. Those commits, and the ones tried and taken back, are in [upstream-commits.md](changes/upstream-commits.md). [upgrading.md](upgrading.md) says how to take one in.

The changes of liustack/tifajs to the C are listed one at a time in [changes](changes/README.md): limits that no script can catch, the same output on every machine, `Intl` in every locale Chromium has, and faster renders.

## Credits

The engine is Fabrice Bellard's and Charlie Gordon's QuickJS. The Go API began as modernc.org/quickjs, by the quickjs-go authors Jan Mercl and Dan Kortschak, who also translated QuickJS to Go with ccgo as modernc.org/libquickjs. liustack/tifajs thanks them for the API it started from. None of their code remains in the repository. QuickJS's license is in [engine/LICENSE](../../engine/LICENSE), also at the root as [LICENSE-QUICKJS](../../LICENSE-QUICKJS).
