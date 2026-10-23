// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Renders tifa SSR pages on a native engine (C QuickJS through `qjs --std`,
// or Node through node-ssr.cjs) the way cmd/jsbench tifa-ssr does: the
// preludes, then the bundle, then for each input one render whose HTML is
// written out, warm renders, and n timed renders. A render is
// startRender(json) followed by the microtasks it queued. Times come from
// performance.now() inside the script. It prints one JSON line.
//
// Arguments: name=value pairs: bundle, inputs (comma list), prelude (comma
// list), html (directory), tag, warm, n, engine.
var driver = (function () {
  function parse(argv) {
    var a = { warm: "50", n: "200", prelude: "", html: "", tag: "", engine: "native" };
    argv.forEach(function (s) { var i = s.indexOf("="); if (i > 0) a[s.slice(0, i)] = s.slice(i + 1); });
    return a;
  }
  function stats(d) {
    var s = d.slice().sort(function (x, y) { return x - y; });
    var sum = 0;
    for (var i = 0; i < s.length; i++) sum += s[i];
    var m = s.length % 2 ? s[(s.length - 1) / 2] : (s[s.length / 2 - 1] + s[s.length / 2]) / 2;
    var ns = function (x) { return Math.round(x * 1e6); };
    return { n: s.length, median_ns: ns(m), min_ns: ns(s[0]), max_ns: ns(s[s.length - 1]), mean_ns: ns(sum / s.length) };
  }
  // host: { read(path), run(code, name), write(path, text), now(), cpu() (ms or null) }
  async function main(host, argv) {
    var a = parse(argv), r = { engine: a.engine, prelude_ms: [], pages: [] };
    try {
      a.prelude.split(",").filter(Boolean).forEach(function (p) {
        var t = host.now();
        host.run(host.read(p), p.split("/").pop());
        r.prelude_ms.push(host.now() - t);
      });
      var t0 = host.now();
      host.run(host.read(a.bundle), a.bundle.split("/").pop());
      r.load_ms = host.now() - t0;
    } catch (e) {
      r.err = "load: " + String(e);
      return r;
    }
    async function render(json) {
      globalThis.renderResult = undefined;
      startRender(json);
      for (var i = 0; globalThis.renderResult === undefined; i++) {
        if (i > 1e6) throw new Error("no renderResult");
        await null;
      }
      var s = globalThis.renderResult;
      globalThis.renderResult = undefined;
      return typeof s === "string" ? s : "not a string: " + String(s);
    }
    var warm = +a.warm, n = +a.n;
    for (var inp of a.inputs.split(",")) {
      var p = { input: inp.split("/").pop(), ok: false };
      r.pages.push(p);
      try {
        var json = host.read(inp);
        var html = await render(json);
        p.html_bytes = html.length;
        if (a.html) host.write(a.html + "/" + a.engine + a.tag + "-" + p.input.replace(/\.json$/, "") + ".html", html);
        for (var i = 0; i < warm; i++) await render(json);
        var d = [], c0 = host.cpu();
        for (var i = 0; i < n; i++) {
          var t = host.now();
          var h = await render(json);
          d.push(host.now() - t);
          if (h.length !== html.length) throw new Error("output changed between renders");
        }
        var c1 = host.cpu();
        if (c0 !== null) p.cpu_ms_per_op = (c1 - c0) / n;
        p.render = stats(d);
        p.ok = true;
      } catch (e) {
        p.err = String(e);
      }
    }
    return r;
  }
  return { main: main };
})();

if (typeof std === "object" && typeof os === "object") {
  // qjs --std ssr-driver.js name=value ...
  var host = {
    read: function (p) { var s = std.loadFile(p); if (s === null) throw new Error("cannot read " + p); return s; },
    run: function (code, name) { std.evalScript(code); },
    write: function (p, s) { var f = std.open(p, "w"); f.puts(s); f.close(); },
    now: function () { return performance.now(); },
    cpu: function () { return null; },
  };
  driver.main(host, scriptArgs.slice(1)).then(function (r) { print(JSON.stringify(r)); },
    function (e) { print(JSON.stringify({ err: String(e) })); });
}
if (typeof module === "object") module.exports = driver;
