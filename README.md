# quickjs (liustack fork)

This repository is a fork of two modernc.org modules, merged into one Go module:

| Upstream module | Upstream repository | Here |
|---|---|---|
| `modernc.org/quickjs` (the Go API) | https://gitlab.com/cznic/quickjs | repository root, package `github.com/liustack/quickjs` |
| `modernc.org/libquickjs` (QuickJS transpiled to Go by ccgo) | https://gitlab.com/cznic/libquickjs | `lib/`, package `github.com/liustack/quickjs/lib` |

It starts from quickjs v0.25.0 and libquickjs v0.13.1 (QuickJS release
2026-06-04, commit `3d5e064`), with these changes:

- A VM that runs out of memory inside `JSON.parse`, or anything else that
  converts a string with non-ASCII characters to UTF-8, no longer leaks that
  string. `JS_ToCStringLen2` kept a reference to its argument when allocating
  the UTF-8 copy failed, so the string outlived `VM.Close`. QuickJS, quickjs-ng
  and libquickjs still had this bug when the fork was made. Test:
  `lib/tocstring_oom_test.go`.
- `VM.Call` and `VM.CallValue` report the exception thrown while evaluating
  the function expression (for example out of memory, or a name that is not
  defined) instead of `cannot call a non-function`.
- Making an error no longer formats its `stack`. The engine records the
  frames (function name, bytecode and position) and formats the text the
  first time `stack` is read, with the same result as before. Code that makes
  many errors and reads few of their stacks, such as the development build of
  React (one error per JSX element), runs much faster: a server render of a
  small React app with that build went from about 240 ms to about 10 ms at
  the median, on darwin/arm64.
  `stack` is an ordinary writable, configurable data property, so the error
  has the same shape as one whose stack was formatted at once, and assigning,
  defining or deleting `stack` costs no more than it does for any string.
  Once read, it is the string. What is different:
  - While the stack is pending, the value of `stack` is an internal object
    that holds the recorded frames, so a pending error carries one extra
    small object. A runtime that records its first lazy stack also registers
    a private class for these objects, a fixed one-time cost of about a
    kilobyte. No code can observe the internal object: every path that reads
    a property value formats the stack first.
  - Until its stack is read or the error is freed, an error keeps the
    bytecode of every function on its stack alive. Most of these functions
    stay alive anyway. A function that runs once and makes an error that
    something keeps is the exception, together with every function it
    defines: one kept error from a function that defines 2,000 others kept
    1.30 MB where 0.30 MB stayed before. Keeping only the file name and the
    line table would need either a copy of the line table for every frame
    of every error, which adds work and memory to every error made, the
    case this change is for, or a reference count of its own for the line
    table, which changes a struct of the engine and so needs a regeneration
    of `lib/`.
  - Errors made while top-level code (a script, an eval or a module body)
    is on the stack are still formatted at once, so that an error a library
    keeps from when it loaded does not keep that code alive. So are errors
    that cannot take new properties and errors that already have a `stack`.
  - If there is no memory left when `stack` is first read, the read throws
    an out of memory error and the stack stays pending, so a later read can
    give the text. Reading the property descriptor, and freezing or sealing
    the error, read the value the same way, so they too format the stack and
    can throw when memory is short. Printing an error (`console.log`) reads
    it without throwing: out of memory there leaves the error with no stack
    and no pending exception.
  - If the text cannot be formatted when a stack is built at once (only a
    function name that needs more memory to convert to UTF-8 can cause this),
    `stack` becomes `null`, in Go and in the C patch alike. The upstream
    engine instead keeps a half-formatted stack with the name shown as
    `<anonymous>`.

  Tests: `errorstack_test.go`, `errorstack_unix_test.go`,
  `lib/backtrace_test.go`, `lib/backtrace_round2_test.go`.

The engine lives in the same module so that a fix to it ships together with
the Go API, without waiting for an upstream release of either module.

```sh
go get github.com/liustack/quickjs
```

The license files of both projects are kept: [LICENSE](LICENSE) and
[LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) at the root, and
[lib/LICENSE](lib/LICENSE), [lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS)
and [lib/LICENSE_TEST262](lib/LICENSE_TEST262) for the engine. Maintenance
notes are at the [end of this file](#maintaining-the-fork).

The rest of this section is the upstream README.

---

[![Go Reference](https://pkg.go.dev/badge/github.com/liustack/quickjs.svg)](https://pkg.go.dev/github.com/liustack/quickjs)
[![LiberaPay](https://liberapay.com/assets/widgets/donate.svg)](https://liberapay.com/jnml/donate)
[![receives](https://img.shields.io/liberapay/receives/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)
[![patrons](https://img.shields.io/liberapay/patrons/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)

![logo_png](logo.png)

# quickjs

Package quickjs is a pure Go embeddable Javascript engine. It supports the
ECMA script 14 ([ES2023]) specification including modules, asynchronous
generators, proxies and BigInt.

See also the original [C QuickJS] library.

The engine has no `Intl`: ECMA-402 is not implemented by the C library this
package wraps. Package [quickjs/intl] adds `Intl.DateTimeFormat` and
`Intl.NumberFormat`, en-US over Go's time zone database, for a VM that opts in
by calling its `Register` — once per VM, including one created to replace
another, because `Intl` lives on that VM's global object.

Resolving IANA time zone names needs a zoneinfo database. Go reads the
operating system's, or the copy `time/tzdata` embeds in the binary. That import
belongs in a program's main package rather than in a library, so neither
package makes it for you.

[C QuickJS]: https://bellard.org/quickjs
[quickjs/intl]: https://pkg.go.dev/github.com/liustack/quickjs/intl
[ES2023]: https://tc39.es/ecma262/2023/

## Maintaining the fork

Branches: `main` carries the fork. `master` and the other upstream branches
mirror gitlab.com/cznic/quickjs and get no commits of their own.

Remotes, set up once in a fresh clone. The engine remotes must not fetch
tags, because libquickjs tags (v0.13.1, ...) would clash with the tags of
this repository:

```sh
git remote add upstream https://gitlab.com/cznic/quickjs.git
git remote add upstream-lib https://gitlab.com/cznic/libquickjs.git
git config remote.upstream-lib.tagOpt --no-tags
git fetch upstream
git fetch upstream-lib 'refs/tags/*:refs/tags/lib/*'   # engine tags as lib/v0.13.2, ...
```

### Taking a new engine release

`lib/` was added with `git subtree`, so an upstream libquickjs release merges
in with its history:

```sh
git subtree pull --prefix lib upstream-lib <libquickjs tag, e.g. v0.13.2>
```

Conflicts to expect:

- `lib/ccgo_*.go`: every upstream release regenerates these files, so the
  two lines added to `XJS_ToCStringLen2` and the calls into
  `lib/backtrace.go` (lazy error stacks) conflict. Take the upstream version
  of every generated file, then put the changes back:

  ```sh
  git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
  go run ./internal/fixlib
  ```

  `fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written
  files under `lib/`. `go run ./internal/fixlib -check` lists files that still
  need changes. If `fixlib` reports that `XJS_ToCStringLen2` changed shape,
  read the new upstream code: QuickJS may have fixed the leak itself. If it
  reports that `_build_backtrace`, `_get_prop_string`,
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, the interpreter's
  `get_field`, `js_print_error`, `js_print_object` or `JS_ComputeMemoryUsage`
  changed shape, compare the new upstream `build_backtrace` with
  `lib/backtrace.go`, which formats the same text from the recorded frames
  and must change with it.
- `lib/internal/patch/quickjs.diff`: holds the same changes in C, so that a
  regeneration keeps them. Keep the `JS_ToCStringLen2` hunk unless upstream
  QuickJS has fixed the bug, and the lazy stack hunks (`JSBacktrace`, the
  marker class, `js_backtrace_build` and the `js_backtrace_*` calls in
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, the interpreter's
  `GET_FIELD_INLINE`, `get_prop_string`, `js_print_error`, `js_print_object`
  and `JS_ComputeMemoryUsage`). After a regeneration that includes them,
  `lib/backtrace.go` and its wiring in `fixlib` are no longer needed.
- `lib/go.mod`, `lib/go.sum`: they do not exist here. When upstream changed
  them, delete them again (`git rm lib/go.mod lib/go.sum`) and bring any newer
  requirement into the root `go.mod`.
- `lib/internal/run-test262/go.mod` is ours. Upstream has none there.
- `lib/internal/run-test262/ccgo_*.go`: ccgo links a whole copy of the
  engine into the test262 runner, so as generated the runner tests that copy
  and not `lib/`. `fixlib` turns each engine API function of the runner into
  a call of the same function in `lib/` (`lib/test262_runner.go` exports the
  one ccgo leaves unexported). Take the upstream files and run `fixlib`, as
  for `lib/ccgo_*.go`.

This fork does not regenerate `lib/ccgo_*.go` itself. `lib/generator.go` still
assumes it is the root of its own module.

### Taking a new release of the Go API

```sh
git merge <upstream tag>
```

Upstream commits import `modernc.org/libquickjs` and `modernc.org/quickjs`.
After the merge, drop `modernc.org/libquickjs` from `go.mod`, run
`grep -rn 'modernc.org/quickjs\|modernc.org/libquickjs' --include='*.go' .`
and switch what it finds to `github.com/liustack/quickjs`. The only expected
leftover is the Symbol key `"modernc.org/quickjs/intl"` in `intl/intl.go`,
which is a runtime marker, not an import path.

### Checks

```sh
go test -short ./...                                     # all packages
go test -vet=off -run 'TestToCString' ./lib              # the leak reproduction
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -vet=off -run 'TestBacktrace' ./lib              # shapes, retries and writes of pending stacks
go run ./internal/fixlib -check                          # fork changes present in lib/
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

`lib/internal/run-test262` is a module of its own, so its 69 MiB of
transpiled test runner stay out of the module zip. `Test262` in `lib/` builds
it with `go build -C`. The runner runs the engine in `lib/` (see `fixlib`),
so it checks the changes made there.
