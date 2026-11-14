---
summary: 'The slowjs command: how to get it prebuilt, how to run a file, what a script has (print, console.log, console.error, scriptArgs), the exit codes, the timeout and memory limit with their defaults and why, how a benchmark harness runs it, what it leaves out on purpose (REPL, modules, stdin, files, network, timers, Node, Intl), the size of the binary with and without Intl, and its tests'
read_when:
  - Running a JavaScript file in this engine without writing Go, or benchmarking the engine from the command line
  - Changing cmd/slowjs, its flags or its release archives
  - Asked why slowjs has no Intl, modules, REPL or Node APIs
---

# The slowjs command

English · [简体中文](command.zh-CN.md)

`slowjs` runs a JavaScript file in this engine, so that you can try it or benchmark it without writing Go. From v0.31.0, the GitHub release of each version has it prebuilt for linux and darwin on amd64 and arm64 and for windows/amd64. The code is `cmd/slowjs`.

## Get it

Download the archive of your system from the [latest release](https://github.com/liustack/quickjs/releases/latest), check it against `checksums.txt` and unpack it:

| System | Archive |
| --- | --- |
| linux/amd64 | `slowjs-x86_64-unknown-linux-gnu.tar.gz` |
| linux/arm64 | `slowjs-aarch64-unknown-linux-gnu.tar.gz` |
| macOS, Apple silicon | `slowjs-aarch64-apple-darwin.tar.gz` |
| macOS, Intel | `slowjs-x86_64-apple-darwin.tar.gz` |
| windows/amd64 | `slowjs-x86_64-pc-windows-msvc.zip` |

```sh
curl -LO https://github.com/liustack/quickjs/releases/latest/download/slowjs-x86_64-unknown-linux-gnu.tar.gz
curl -LO https://github.com/liustack/quickjs/releases/latest/download/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf slowjs-x86_64-unknown-linux-gnu.tar.gz
./slowjs-x86_64-unknown-linux-gnu/slowjs --version
```

Each archive holds one folder with the binary and the license texts it must carry ([NOTICE.md](../NOTICE.md#what-a-program-that-ships-a-binary-must-include)). The binary is static, built with `CGO_ENABLED=0`: the linux ones run on glibc and musl systems alike. macOS keeps a binary a browser downloaded from running, as it is not signed by a developer: `curl` sets no such mark, and `xattr -d com.apple.quarantine slowjs` removes it.

## Run a file

```sh
slowjs [flags] file.js [args...]
slowjs [flags] -e code [args...]
```

| Flag | What it does |
| --- | --- |
| `-e code` | Run `code` instead of a file |
| `--timeout d` | End the run after `d`, such as `30s` or `2m`. `0` never ends it. Default `10m` |
| `--memory-limit n` | End the run when the VM takes more than `n` bytes. `K`, `M` and `G` (or `KiB`, `MiB`, `GiB`) are 1024, 1024² and 1024³. `0` is no limit. Default `2GiB` |
| `--version` | Print `slowjs <version>` on one line and exit |
| `-h`, `--help` | Print the flags and exit |

The file runs as a global script (not a module) in a new VM, then the promise jobs it queued run, so `then` callbacks and async functions finish. The script has the language as the engine runs it (ES2023, with BigInt, proxies, generators and async functions), and these:

- `print(...args)` and `console.log(...args)`: each argument made a string as `String(x)` makes it, joined by spaces, and a newline, to stdout. As in `qjs`, an object prints as `[object Object]`. Both work called on their own, as `var p = console.log; p(1)`.
- `console.error(...args)`: the same, to stderr.
- `scriptArgs`: the arguments after the flags, as in `qjs`: the file and what follows it, or with `-e` only what follows.

Each call writes its line at once, and stdout is written before a line of stderr, so a terminal or a harness reading a pipe sees the lines when the script prints them, in that order. A lone surrogate prints as U+FFFD.

An uncaught exception prints its text and its stack to stderr, at most 50 frames. The file in the stack is `<eval>`, the name under which the Go API evaluates a script, not the name of the file.

## Exit codes

| Code | When |
| --- | --- |
| 0 | The script and its jobs ran to the end |
| 1 | An uncaught exception, a syntax error included |
| 2 | A wrong command line, or the file cannot be read. Nothing ran |
| 3 | The run passed `--timeout` |
| 4 | The VM passed `--memory-limit` |

A crash of Go itself also ends with 2, and prints `panic:` or `fatal error:` and the goroutines.

## The limits

A timeout or the memory limit ends the run whatever the script does: no `catch`, `finally` or promise handler runs after it, as with every call of the Go API ([uncatchable.md](changes/uncatchable.md)).

- **The timeout** covers the whole run, the script and its jobs, not each. Its default of 10 minutes is there so that a script that never ends does not hold a machine that runs it unattended, such as a benchmark job with no timeout of its own, for hours. It is more than 10 times what a run of the V8 benchmark suite takes (36 seconds in one run on a busy Apple M4, 31 to 47 seconds for C QuickJS and QuickJS-NG on the runners of js-engine-benchmark) and 4 times the slowest engine of that table (152 seconds). A longer workload sets `--timeout 0` or a longer time.
- **The memory limit** counts what the VM takes: its objects, strings and the engine's own structures. Its default of 2 GiB is there so that a script that allocates without end stops at an error, before the system runs out of memory and swaps or ends another process. It is 15 times what the V8 suite needs: the suite runs out of memory at `--memory-limit 128MiB` and runs to the end with no limit, with a peak of 141 MB resident (Apple M4, one run each). A workload that needs more sets `--memory-limit 0` or a larger size. A limit below 128 KiB counts as 128 KiB.
- `print` and `console` hand a line to Go in pieces of at most 65,536 UTF-16 units, so the copy Go makes to write it is at most 192 KiB at a time, outside the limit, however long the string the script made under it. Outside the limit too: the source file, which Go reads whole before the run, and the memory of Go's runtime.

## In a benchmark harness

A harness that runs engines from the command line, such as [js-engine-benchmark](https://github.com/ahaoboy/js-engine-benchmark), needs four things, and slowjs has them:

- It runs `slowjs <file>` with the suite in one file: run.js of the V8 suite with each `load('x.js')` replaced by the file. slowjs has no `load`.
- The suite prints its results with `print`, or with `console.log` when there is one, called on its own (`var print = console.log`), and the harness reads `Name: score` lines from stdout.
- It reads the version from `--version` with a pattern such as `v([\d.]+...)`, so the version is on one line.
- An installer of prebuilt binaries (js-engine-benchmark uses easy-install) finds the archive of a system by the target name in the file name, as Rust names targets.

To run the V8 suite that `compare/engines` uses, in `internal/arewefastyet/v8-v7`, the same way:

```sh
cd internal/arewefastyet/v8-v7
for f in base richards deltablue crypto raytrace earley-boyer regexp splay navier-stokes; do cat $f.js; echo; done >/tmp/v8.js
sed -n '/^var success/,$p' run.js >>/tmp/v8.js
slowjs /tmp/v8.js
```

## Left out on purpose

The command is the smallest one that runs a file the way the harnesses above run engines. What it leaves out:

- **No REPL.** With no file and no `-e`, it prints how to use it and exits with 2.
- **No ES modules.** `import` and `export` are a syntax error in a global script, and the Go API has no module loader since v0.27.0 ([go-api.md](changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)). None of the harnesses read needs them: js-engine-benchmark runs one plain script.
- **No stdin.** No harness read passes the script on stdin.
- **No file system, network, timers or Node APIs.** There is no `load`, `require`, `process`, `fs`, `setTimeout` or `fetch`. It is an engine, not Node.
- **No Intl or Temporal.** A script that uses `Intl` gets a ReferenceError, and `toLocaleString` and `localeCompare` behave as in QuickJS without Intl. The data of every Chromium locale would make the binary 2.6 times as large (below), and no benchmark of the V8 suite needs it. A Go program that wants them registers `intl` and `temporal` on its VM ([api.md](api.md)).
- **No report of an unhandled promise rejection.** A rejected promise that nothing handles ends the run with 0, as the Go API has no hook that tells.

Size of the binary, built as the release builds it (`-trimpath -ldflags "-s -w"`, Go 1.27.2), in MiB:

| Target | slowjs | with `intl` | with `intl` and `temporal` |
| --- | --: | --: | --: |
| linux/amd64 | 7.1 | 18.0 | 18.7 |
| linux/arm64 | 6.5 | 17.1 | 17.8 |
| darwin/amd64 | 7.2 | 18.1 | 18.8 |
| darwin/arm64 | 6.5 | 17.3 | 17.9 |
| windows/amd64 | 7.4 | 18.3 | 19.0 |

The archives are 2.3 to 2.7 MiB. Compressed with gzip, the linux/amd64 binary is 2.6 MiB alone and 8.1 MiB with `intl`.

## Tests

`go test ./cmd/slowjs` builds the command with a version stamped in and runs it on small scripts: what `print` and `console` write and in what order, a long string with a surrogate pair across two pieces, `scriptArgs`, an uncaught exception with its stack, a syntax error, promise jobs, the timeout ending an endless loop that catches (and a run whose jobs pass it), the memory limit ending a loop that allocates, `--version`, `-h` and the wrong command lines. `TestV8Suite`, which `-short` skips, runs the V8 suite through the command as a harness runs it, and checks that each benchmark gives a score and that the benchmarks whose own checks pass are those that pass in a VM of the Go API. It times nothing.

With `SLOWJS_BIN` and `SLOWJS_VERSION` set, the tests run that binary instead of building one, and expect that version. The release job runs them so against each binary it built, on its own system ([releasing.md](releasing.md#the-release-job)).
