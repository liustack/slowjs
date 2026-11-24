---
name: Bug report
about: A script gave a wrong answer, crashed, ran slow, or a limit did not hold
title: ''
labels: bug
assignees: ''
---

A security problem (memory outside the VM, a crash of the Go process, a limit a script got past) goes in a [private report](https://github.com/liustack/beastjs/security/advisories/new), not here.

## What happened

A clear description of the problem.

## The script that shows it

A JavaScript file, cut down as far as it still shows the problem, and the exact command you ran it with. `beastjs` runs a file in the engine, set up as tifa sets it up, with no Go to write: take it from the release, see [docs/use/command.md](https://github.com/liustack/beastjs/blob/main/docs/use/command.md).

```js
...
```

```sh
beastjs repro.js
```

For a problem of the Go API (a call from Go, a Value, a Builtin, a limit set from Go), give the Go program instead, cut down the same way.

```go
vm, _ := beastjs.NewVM()
defer vm.Close()
v, err := vm.Eval(`...`, beastjs.EvalGlobal)
```

## The full output

Paste the complete output, not a paraphrase: what `beastjs` printed and its exit code (`echo $?`), or what the Go program printed. For a crash, the whole Go stack trace.

```
...
```

## Environment

- `beastjs --version`:
- For a Go program instead: the version (`go list -m github.com/liustack/beastjs`), `go version`, and the packages registered on the VM (`intl`, `temporal`, `url`, `fmt`):
- OS and CPU model:

## Expected

What you expected instead. If Node or C QuickJS gives a different answer for the same script, say which and what it gives.

---

Not sure it is a bug, or just want to talk it through? Find me on X: [@liustack](https://x.com/liustack).
