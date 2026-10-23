// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Records Node's answers for the "node" probes into features.json, which the
// Go runner embeds. Run with: node gen.mjs
import vm from "node:vm";
import { writeFileSync } from "node:fs";
import { features } from "./features.src.js";

const out = features.map(([id, edition, kind, src]) => {
  const f = { id, edition, kind, src, expect: "ok" };
  if (kind === "node") {
    f.expect = String(vm.runInContext(src, vm.createContext({})));
    f.kind = "script";
  }
  if (kind === "host") f.expect = "";
  return f;
});
writeFileSync("features.json", JSON.stringify({ node: process.version, icu: process.versions.icu, features: out }, null, 1) + "\n");
console.log(out.length, "features; node", process.version, "icu", process.versions.icu);
