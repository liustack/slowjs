---
title: Using beastjs
audience: user
order: 1
---

# Using beastjs

English · [简体中文](index.zh-CN.md)

For Go programmers who run JavaScript inside a Go program with `github.com/liustack/beastjs`, and for anyone who runs a script with the `beastjs` command.

- [The Go API beyond Eval and Call](api.md): find the function for calling Go from JavaScript, ending a call, keeping a VM's memory down, setting its clock, and rendering React faster, with the release each one came in.
- [The beastjs command](command.md): run a script on the engine, or show a problem with it, without writing Go.
- [JavaScript engines compared](benchmarks.md): see how fast the engine renders pages and scores on the V8 suite next to the other engines for Go and three native interpreters, and how to run the comparison yourself.
- [Moving from modernc.org/quickjs](from-modernc.md): know what to change in your code and `go.mod`, and which behaviour differs.
