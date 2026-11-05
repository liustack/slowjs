# 安全策略

[English](SECURITY.md) · 简体中文

## 报告漏洞

安全问题请私下报告，不要开公开 issue。

用 [GitHub Security Advisories](https://github.com/liustack/quickjs/security/advisories/new) 提交私密报告。修复发布之前，细节只在你和维护者之间。

算安全问题的情形：脚本读写了自己 VM 之外的内存，让 Go 进程崩溃，越过了 eval 超时或内存上限还在跑，碰到了宿主没给它的东西，或者让一个 VM 看到另一个 VM。请附上能复现的最小脚本和 Go 代码、完整输出、liustack/quickjs 版本（`go list -m github.com/liustack/quickjs`）、`go version`，以及操作系统和 CPU。你会收到回复和修复，除非你不愿意，否则会署上你的名字致谢。

## 支持的版本

修复会发在最新次版本的新补丁版本里，报告前请先升级到最新版本。能用哪些 Go 版本构建见 [README](README.zh-CN.md#安装)。

## 需要知道的

引擎只运行你交给它的脚本，脚本能用的只有你通过 Go API 给它的东西：它自己不碰文件系统、网络和进程。你设的限制（`SetEvalTimeout`、`SetMemoryLimit`）不管脚本怎么 catch 都会结束这次调用。一个 VM 不能同时给多个 goroutine 用，这样共用 VM 的代码不在引擎承诺的范围内（见 [docs/architecture.zh-CN.md](docs/architecture.zh-CN.md)）。
