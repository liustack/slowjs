# Security policy

English · [简体中文](SECURITY.zh-CN.md)

## Reporting a vulnerability

Report a security problem privately, not in a public issue.

Use [GitHub Security Advisories](https://github.com/liustack/quickjs/security/advisories/new) to open a private report. The details stay between you and the maintainer until a fix is out.

What counts: a script that reads or writes memory outside its VM, crashes the Go process, runs on past its eval timeout or its memory limit, reaches anything the host did not give it, or makes one VM see another. Include the smallest script and Go code that show it, the full output, the liustack/quickjs version (`go list -m github.com/liustack/quickjs`), `go version`, and the OS and CPU. You will get an answer, a fix, and credit unless you prefer otherwise.

## Supported versions

Fixes land in a new patch release of the latest minor version, so upgrade to the newest release before reporting. The Go versions it builds with are in the [README](README.md#install).

## What to keep in mind

The engine runs the scripts you give it, with what you give them through the Go API, and nothing else: no file system, network or process of its own. A limit you set (`SetEvalTimeout`, `SetMemoryLimit`) ends the call whatever the script catches. A VM is not safe for several goroutines at once, and code that shares one that way is outside what the engine promises ([docs/architecture.md](docs/architecture.md)).
