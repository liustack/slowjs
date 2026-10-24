// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// node node-ssr.cjs name=value ...: ssr-driver.js on Node. The bundle and
// the preludes run as global scripts, as on the Go engines.
const fs = require("fs");
const vm = require("vm");
const driver = require("./ssr-driver.js");
// Bundles may replace console, so keep a direct way to stdout.
const out = (s) => fs.writeSync(1, s + "\n");
// Node defines some web globals as getters (crypto, navigator). The host
// patches of a tifa bundle assign them, so make them plain values first.
for (const [k, d] of Object.entries(Object.getOwnPropertyDescriptors(globalThis))) {
  if (d.get && d.configurable) Object.defineProperty(globalThis, k, { value: globalThis[k], writable: true, configurable: true, enumerable: d.enumerable });
}
// Node v24.11.1 has no Iterator.concat (ES2026), which TanStack Router calls.
if (!Iterator.concat) Iterator.concat = (...its) => Iterator.from((function* () { for (const it of its) yield* it; })());
const host = {
  read: (p) => fs.readFileSync(p, "utf8"),
  run: (code, name) => vm.runInThisContext(code, { filename: name }),
  write: (p, s) => fs.writeFileSync(p, s),
  now: () => performance.now(),
  cpu: () => { const u = process.cpuUsage(); return (u.user + u.system) / 1000; },
};
driver.main(host, process.argv.slice(2)).then((r) => out(JSON.stringify(r)), (e) => out(JSON.stringify({ err: String(e) })));
