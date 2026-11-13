# Data of web-platform-tests

`wpt/*.any.js` are copied unchanged from `url/` of
github.com/web-platform-tests/wpt at commit
`8af128e89e4d51dd18298361058d3616bbe81afc` (2026-11-12). The JSON they fetch
is `../../internal/whatwgurl/testdata/` (see its SOURCES.md). `harness.js` is
ours: the few functions of testharness.js these files use. The license of
web-platform-tests is in [WPT-LICENSE.md](WPT-LICENSE.md).

`known-failures.txt` is ours: the tests a VM with this package does not pass,
each with its reason.
