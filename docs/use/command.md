---
summary: 'The tifajs command: what it is for (run a script on the engine, show a problem, try a release without Go), how to get it, what a script has (Intl, Temporal, URL, fmt, print, console, scriptArgs), the exit codes, the timeout and memory limit with their defaults and why, what it leaves out on purpose (timers, modules, REPL, files, network, a report of unhandled rejections), its size and its tests'
audience: user
order: 3
read_when:
  - Running a JavaScript file in this engine without writing Go, or showing a problem of the engine with a script
  - Changing cmd/tifajs, its flags or its release archives
  - Asked why tifajs has no timers, modules, REPL or Node APIs
---

# The tifajs command

English · [简体中文](command.zh-CN.md)

`tifajs` runs a JavaScript file in this engine, in a VM set up as tifa sets it up, with no Go program to write. It is for three things:

- **Show a problem.** A script and what `tifajs` printed for it is the whole report: the engine is the one we ship, set up as we use it.
- **Run a script on the engine**, to see what it gives, how long it takes or how much memory it needs.
- **Try a release without Go.** From v0.31.0, the GitHub release of each version has it prebuilt for linux and darwin, on amd64 and arm64, and for windows/amd64, and from v0.31.12 for windows/arm64 too.

The code is `cmd/tifajs`.

## Get it

Download the archive of your system from the [latest release](https://github.com/liustack/tifajs/releases/latest), check it against `checksums.txt` and unpack it. The archives are named after the target as Go names it: `tifajs-linux-amd64.tar.gz`, `tifajs-linux-arm64.tar.gz`, `tifajs-darwin-amd64.tar.gz` (macOS, Intel), `tifajs-darwin-arm64.tar.gz` (macOS, Apple silicon), `tifajs-windows-amd64.zip` and `tifajs-windows-arm64.zip`.

```sh
curl -LO https://github.com/liustack/tifajs/releases/latest/download/tifajs-linux-amd64.tar.gz
curl -LO https://github.com/liustack/tifajs/releases/latest/download/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf tifajs-linux-amd64.tar.gz
./tifajs-linux-amd64/tifajs --version
```

Each archive holds one folder with the binary and the license texts it must carry ([NOTICE.md](../../NOTICE.md#what-a-program-that-ships-a-binary-must-include)). The binary is static, built with `CGO_ENABLED=0`: the linux ones run on glibc and musl systems alike. macOS keeps a binary a browser downloaded from running, as it is not signed by a developer: `curl` sets no such mark, and `xattr -d com.apple.quarantine tifajs` removes it.

## Run a file

```sh
tifajs [flags] file.js [args...]
tifajs [flags] -e code [args...]
```

| Flag | What it does |
| --- | --- |
| `-e code` | Run `code` instead of a file |
| `--timeout d` | End the run after `d`, such as `30s` or `2m`. `0` never ends it. Default `1m` |
| `--memory-limit n` | End the run when the VM takes more than `n` bytes. `K`, `M` and `G` (or `KiB`, `MiB`, `GiB`) are 1024, 1024² and 1024³. `0` is no limit. Default `1GiB` |
| `--version` | Print `tifajs <version> <go release> <os>/<arch>` on one line and exit, such as `tifajs v0.31.0 go1.27.2 linux/amd64` |
| `-h`, `--help` | Print the flags and exit |

The file runs as a global script (not a module) in a new VM, then the promise jobs it queued run, so `then` callbacks and async functions finish.

## What a script has

The VM is set up as tifa's renderer sets up each of its VMs, in the same order, after the timeout is set:

- calls may nest 10000 deep, tifa's default and the most the engine allows;
- `URL` and `URLSearchParams` (package `url`);
- `Intl`, in every locale Chromium has, and the `toLocaleString` methods and `localeCompare` that use it (package `intl`);
- `Temporal` (package `temporal`).

Then, which tifa does not have:

- `fmt`: Go's `fmt.printf`, `fmt.println`, `fmt.sprintf` and `fmt.sprint` (package `fmt`). `printf` and `println` write to stdout.
- `print(...args)` and `console.log(...args)`: each argument made a string as `String(x)` makes it, joined by spaces, and a newline, to stdout. An object prints as `[object Object]`. Both work called on their own, as `var p = console.log; p(1)`.
- `console.error(...args)`: the same, to stderr.
- `scriptArgs`: the arguments after the flags: the file and what follows it, or with `-e` only what follows.

A script also sees the functions named `__tifa_*` that tifa gives its bundles, since the command sets the VM up with `tifa.Register` ([api.md](api.md#rendering-react-faster)).

Each call writes its line at once, and stdout is written before a line of stderr, so a terminal or a pipe sees the lines when the script prints them, in that order. A lone surrogate prints as U+FFFD.

An uncaught exception prints its text and its stack to stderr, at most 50 frames. The stack names the file by its path as given on the command line, cleaned to one line of at most 1,024 bytes, and the code of `-e` as `<eval>`.

## Show a problem

1. Cut the script down as far as it still shows the problem.
2. Run it: `tifajs repro.js; echo $?`.
3. Report the script, the command, everything it printed, the exit code and the line of `tifajs --version` (the bug report template asks for them).

With the default limits, a reproduction that hangs ends after a minute with exit code 3, and one that allocates without end ends at 1 GiB with exit code 4, so a run never has to be killed by hand.

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

A timeout or the memory limit ends the run whatever the script does: no `catch`, `finally` or promise handler runs after it, as with every call of the Go API ([uncatchable.md](../internals/changes/uncatchable.md)).

- **The timeout** covers the whole run, the script and its jobs, not each. Its default of one minute is for a person who runs a script and waits for it: a script that hangs ends while they still wait, with a message that names the flag. A tifa page renders in milliseconds and tifa ends a render after 3 seconds, so a minute leaves room for a script far heavier than a page. A longer run sets `--timeout 0` or a longer time.
- **The memory limit** counts what the VM takes: its objects, strings and the engine's own structures. Its default of 1 GiB stops a script that allocates without end at an error, before the machine runs out of memory and swaps. tifa gives each VM 128 MiB, so 1 GiB leaves room for a script eight times larger than a page may be. A run that needs more sets `--memory-limit 0` or a larger size. A limit below 128 KiB counts as 128 KiB.
- `print` and `console` hand a line to Go in pieces of at most 65,536 UTF-16 units, so the copy Go makes to write it is at most 192 KiB at a time, outside the limit, however long the string the script made under it. `fmt` bounds its output under the limit itself (see the doc comment of `fmt.Register`). Outside the limit too: the source file, which Go reads whole before the run, and the memory of Go's runtime.

## Left out on purpose

The command runs what the engine runs in tifa and in a Go program, and nothing a host would have to add:

- **No timers.** `setTimeout` and `setInterval` are a ReferenceError. A render of tifa runs no timer and a VM of the Go API has none, so a problem of either shows without one. Promise jobs do run.
- **No ES modules.** `import` and `export` are a syntax error in a global script, and the Go API has no module loader since v0.27.0 ([go-api.md](../internals/changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)).
- **No REPL and no stdin.** With no file and no `-e`, it prints how to use it and exits with 2.
- **No file system, network or Node APIs.** There is no `load`, `require`, `process`, `fs` or `fetch`. It is the engine, not Node.
- **No report of an unhandled promise rejection.** A rejected promise that nothing handles ends the run with 0: the command does not ask the VM to track them (`VM.TrackRejections`).

Size of the binary, built as the release builds it (`-trimpath -ldflags "-s -w"`, Go 1.27.2), in MiB:

| Target | tifajs |
| --- | --: |
| linux/amd64 | 19.6 |
| linux/arm64 | 18.5 |
| darwin/amd64 | 19.7 |
| darwin/arm64 | 18.7 |
| windows/amd64 | 19.9 |
| windows/arm64 | 18.7 |

Most of it is the locale data of `intl`. The archives are 8.3 to 8.9 MiB.

## Tests

`go test ./cmd/tifajs` builds the command with a version stamped in and runs it on small scripts: what `print` and `console` write and in what order, a long string with a surrogate pair across two pieces, `scriptArgs`, an uncaught exception with its stack, a syntax error, promise jobs, `Intl`, `Temporal`, `URL` and `fmt`, the depth of calls, the timeout ending an endless loop that catches (and a run whose jobs pass it), the memory limit ending a loop that allocates, the defaults, `--version`, `-h` and the wrong command lines. `TestV8Suite`, which `-short` skips, runs a long script that uses much of the engine through the command with no timeout, and checks what it prints against a VM of the Go API.

With `TIFAJS_BIN` and `TIFAJS_VERSION` set, the tests run that binary instead of building one, and expect that version. The release job runs them so against each binary it built, on its own system ([releasing.md](../internals/releasing.md#the-release-job)).
