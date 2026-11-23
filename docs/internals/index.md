---
title: Working on tifajs
audience: dev
order: 2
---

# Working on tifajs

For people who change the engine, its Go API or its tools.

- [Architecture](architecture.md): the layers from the Go API down to the C engine, what each folder holds, the core types and their rules, and the platforms.
- [Changes](changes/README.md): every change tifajs makes to the QuickJS C tree and to the Go API, by area, with its release and why it is safe.
- [Testing](testing.md): the checks to run before a commit, test262, the sanitizers, fuzzing, the render stress and what CI runs.
- [Commit rules](commit.md): the message format, atomic commits and changes to the generated engine.
- [Releasing](releasing.md): versions, tags, the release job and what follows a release.
- [Upgrading](upgrading.md): taking a commit of upstream QuickJS, a newer Go, and new data for `Intl`.
- [Upstream](upstream.md): where the code comes from and who wrote it.
- [Allocator](allocator.md): the memory allocator of the C engine and what the memory limit counts.
- [Measuring](measuring.md): how to tell whether a change makes renders faster or slower, down to a fraction of a percent.
- [Performance](performance.md): what each release gained, how to measure in tifa, and what was tried and not done.
