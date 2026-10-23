// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Bundles app.jsx into one IIFE script that defines globalThis.render.
// react-dom/server is pointed at React's legacy browser build, which has
// renderToString and needs no host functions; the package's server.browser
// entry also loads the streaming renderer, which makes a TextEncoder and a
// MessageChannel when it loads.
import * as esbuild from "esbuild";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { createRequire } from "node:module";
import vm from "node:vm";

const require = createRequire(import.meta.url);
const legacy = join(dirname(require.resolve("react-dom/package.json")), "cjs/react-dom-server-legacy.browser.production.js");

for (const [target, out] of [["es2020", "react-ssr.js"], ["es5", "react-ssr.es5.js"]]) {
  try {
    await esbuild.build({
      entryPoints: ["app.jsx"],
      bundle: true,
      format: "iife",
      minify: true,
      target,
      jsx: "automatic",
      define: { "process.env.NODE_ENV": '"production"' },
      alias: { "react-dom/server": legacy },
      outfile: out,
      logLevel: "silent",
    });
    console.log("built", out);
  } catch (e) {
    console.log("cannot build", out, "for", target + ":", e.errors?.[0]?.text ?? e.message);
  }
}

// The reference output: run the bundle in a fresh Node context.
const src = readFileSync("react-ssr.js", "utf8");
const ctx = vm.createContext({});
vm.runInContext(src, ctx);
const html = ctx.render();
writeFileSync("react-ssr.expected.html", html);
const sum = (s) => createHash("sha256").update(s).digest("hex");
writeFileSync("react-ssr.sha256", `${sum(src)}  react-ssr.js\n${sum(html)}  react-ssr.expected.html\n`);
console.log("html bytes", Buffer.byteLength(html), "elements", (html.match(/<[a-z]/g) || []).length);
