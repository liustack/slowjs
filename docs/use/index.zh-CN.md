---
title: 使用 slowjs
audience: user
order: 1
translatedFrom: 46f288c9ce36
---

# 使用 slowjs

[English](index.md) · 简体中文

给用 `github.com/liustack/slowjs` 在 Go 程序里跑 JavaScript 的 Go 开发者，也给用 `slowjs` 命令跑脚本的人。

- [Eval 和 Call 之外的 Go API](api.zh-CN.md)：找到从 JavaScript 调用 Go、结束一次调用、压低 VM 内存、设置 VM 时钟和加快 React 渲染要用的函数，每个都写了是哪个版本加入的。
- [slowjs 命令](command.zh-CN.md)：不写 Go，也能在引擎上跑脚本，或者用脚本复现引擎的问题。
- [JavaScript 引擎对比](benchmarks.zh-CN.md)：看引擎渲染页面的速度和 V8 测试套件的分数，和其他 Go 引擎以及三个原生解释器相比如何，以及怎样自己重跑这个对比。
- [从 modernc.org/quickjs 迁移](from-modernc.zh-CN.md)：弄清代码和 `go.mod` 要改什么，哪些行为不一样。
