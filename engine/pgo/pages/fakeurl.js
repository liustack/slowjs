// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// A small URL and URLSearchParams for the native test host, which has none
// (they come from the Go package url, and tifa's bundles no longer carry
// core-js's). It parses an absolute URL with a scheme and a host, and resolves
// a path against a base, which is what the pages of engine/pgo/pages ask of it.
// It does not normalize or encode, so it is no parser for other input.
(function () {
  if (typeof globalThis.URL === 'function') return;
  var plus = /\+/g;
  function decode(s) {
    s = s.replace(plus, ' ');
    try { return decodeURIComponent(s); } catch (e) { return s; }
  }
  function encode(s) {
    return encodeURIComponent(s).replace(/%20/g, '+');
  }

  function URLSearchParams(init) {
    this._list = [];
    this._url = null;
    if (init === undefined || init === null) return;
    if (typeof init === 'object') {
      if (typeof init[Symbol.iterator] === 'function') {
        for (var pair of init) this._list.push([String(pair[0]), String(pair[1])]);
      } else {
        for (var k of Object.keys(init)) this._list.push([k, String(init[k])]);
      }
      return;
    }
    this._parse(String(init));
  }
  URLSearchParams.prototype._parse = function (s) {
    this._list = [];
    if (s.charAt(0) === '?') s = s.slice(1);
    if (s === '') return;
    var parts = s.split('&');
    for (var i = 0; i < parts.length; i++) {
      if (parts[i] === '') continue;
      var j = parts[i].indexOf('=');
      if (j < 0) this._list.push([decode(parts[i]), '']);
      else this._list.push([decode(parts[i].slice(0, j)), decode(parts[i].slice(j + 1))]);
    }
  };
  URLSearchParams.prototype._update = function () {
    if (this._url) {
      var q = this.toString();
      this._url._search = q === '' ? '' : '?' + q;
    }
  };
  URLSearchParams.prototype.append = function (k, v) { this._list.push([String(k), String(v)]); this._update(); };
  URLSearchParams.prototype.delete = function (k) {
    k = String(k);
    this._list = this._list.filter(function (p) { return p[0] !== k; });
    this._update();
  };
  URLSearchParams.prototype.get = function (k) {
    k = String(k);
    for (var i = 0; i < this._list.length; i++) if (this._list[i][0] === k) return this._list[i][1];
    return null;
  };
  URLSearchParams.prototype.getAll = function (k) {
    k = String(k);
    return this._list.filter(function (p) { return p[0] === k; }).map(function (p) { return p[1]; });
  };
  URLSearchParams.prototype.has = function (k) { return this.get(k) !== null; };
  URLSearchParams.prototype.set = function (k, v) {
    k = String(k); v = String(v);
    var out = [], done = false;
    for (var i = 0; i < this._list.length; i++) {
      if (this._list[i][0] !== k) out.push(this._list[i]);
      else if (!done) { out.push([k, v]); done = true; }
    }
    if (!done) out.push([k, v]);
    this._list = out;
    this._update();
  };
  URLSearchParams.prototype.sort = function () {
    this._list.sort(function (a, b) { return a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0; });
    this._update();
  };
  URLSearchParams.prototype.forEach = function (f, self) {
    for (var i = 0; i < this._list.length; i++) f.call(self, this._list[i][1], this._list[i][0], this);
  };
  URLSearchParams.prototype.entries = function () { return this._list.map(function (p) { return [p[0], p[1]]; })[Symbol.iterator](); };
  URLSearchParams.prototype.keys = function () { return this._list.map(function (p) { return p[0]; })[Symbol.iterator](); };
  URLSearchParams.prototype.values = function () { return this._list.map(function (p) { return p[1]; })[Symbol.iterator](); };
  URLSearchParams.prototype[Symbol.iterator] = URLSearchParams.prototype.entries;
  URLSearchParams.prototype.toString = function () {
    return this._list.map(function (p) { return encode(p[0]) + '=' + encode(p[1]); }).join('&');
  };
  Object.defineProperty(URLSearchParams.prototype, 'size', { get: function () { return this._list.length; } });

  var absolute = /^([a-zA-Z][a-zA-Z0-9+.-]*:)\/\/([^/?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/;
  var relative = /^([^?#]*)(\?[^#]*)?(#.*)?$/;

  function URL(input, base) {
    input = String(input);
    var m = absolute.exec(input);
    if (!m) {
      if (base === undefined) throw new TypeError('Invalid URL: ' + input);
      var b = new URL(base), r = relative.exec(input);
      var path = r[1], search = r[2], hash = r[3];
      if (path === '') {
        path = b.pathname;
        if (search === undefined) search = b.search;
      } else if (path.charAt(0) !== '/') {
        path = b.pathname.slice(0, b.pathname.lastIndexOf('/') + 1) + path;
      }
      m = [input, b.protocol, b.host, path, search, hash];
    }
    this._protocol = m[1].toLowerCase();
    this._host = m[2];
    this._pathname = m[3] || '/';
    this._search = m[4] === '?' ? '' : (m[4] || '');
    this._hash = m[5] === '#' ? '' : (m[5] || '');
    this._params = null;
  }
  function getter(name, get, set) {
    Object.defineProperty(URL.prototype, name, { get: get, set: set, configurable: true });
  }
  getter('protocol', function () { return this._protocol; });
  getter('host', function () { return this._host; });
  getter('hostname', function () { return this._host.replace(/:\d*$/, ''); });
  getter('port', function () { var m = /:(\d*)$/.exec(this._host); return m ? m[1] : ''; });
  getter('origin', function () { return this._protocol + '//' + this._host; });
  getter('pathname', function () { return this._pathname; }, function (v) { v = String(v); this._pathname = v.charAt(0) === '/' ? v : '/' + v; });
  getter('search', function () { return this._search; }, function (v) {
    v = String(v);
    this._search = v === '' || v === '?' ? '' : (v.charAt(0) === '?' ? v : '?' + v);
    if (this._params) this._params._parse(this._search);
  });
  getter('hash', function () { return this._hash; }, function (v) {
    v = String(v);
    this._hash = v === '' || v === '#' ? '' : (v.charAt(0) === '#' ? v : '#' + v);
  });
  getter('searchParams', function () {
    if (!this._params) {
      this._params = new URLSearchParams(this._search);
      this._params._url = this;
    }
    return this._params;
  });
  getter('username', function () { return ''; });
  getter('password', function () { return ''; });
  getter('href', function () { return this.origin + this._pathname + this._search + this._hash; });
  URL.prototype.toString = URL.prototype.toJSON = function () { return this.href; };
  URL.canParse = function (input, base) {
    try { new URL(input, base); return true; } catch (e) { return false; }
  };

  globalThis.URL = URL;
  globalThis.URLSearchParams = URLSearchParams;
})();
