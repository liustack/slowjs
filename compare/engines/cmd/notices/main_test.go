// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func tarball(files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, name := range []string{"package/package.json", "package/LICENSE", "package/NOTICE.txt", "package/src/LICENSE", "package/index.js"} {
		body, ok := files[name]
		if !ok {
			continue
		}
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	zw.Close()
	return buf.Bytes()
}

// tarballs are the two packages of the polyfill lockfile of the fixture:
// polyfill-a needs polyfill-b, both with a license, b also with a notice.
func tarballs() map[string][]byte {
	return map[string][]byte{
		"/a.tgz": tarball(map[string]string{
			"package/package.json": `{"name":"polyfill-a","version":"1.0.0","license":"MIT"}`,
			"package/LICENSE":      "MIT License\n\nCopyright a  \n",
			"package/src/LICENSE":  "not at the top\n",
		}),
		"/b.tgz": tarball(map[string]string{
			"package/package.json": `{"name":"@fmt/polyfill-b","version":"2.1.0","license":"MIT"}`,
			"package/LICENSE":      "MIT License\n\nCopyright a  \n",
			"package/NOTICE.txt":   "Notice b\r\n",
		}),
	}
}

func integrity(b []byte) string {
	s := sha512.Sum512(b)
	return "sha512-" + base64.StdEncoding.EncodeToString(s[:])
}

// buildFixture writes a tifa checkout in root and the pinned files in here,
// with the tarballs of the lockfile served at base.
func buildFixture(root, here, base string) error {
	tbs := tarballs()
	lock := fmt.Sprintf(`{"packages": {
 "": {"dependencies": {"polyfill-a": "1", "esbuild": "0.28.2"}},
 "node_modules/polyfill-a": {"version": "1.0.0", "resolved": "%[1]s/a.tgz", "integrity": "%[2]s", "license": "MIT", "dependencies": {"@fmt/polyfill-b": "2"}},
 "node_modules/@fmt/polyfill-b": {"version": "2.1.0", "resolved": "%[1]s/b.tgz", "integrity": "%[3]s", "license": "MIT"},
 "node_modules/esbuild": {"version": "0.28.2", "resolved": "%[1]s/none.tgz", "integrity": "sha512-AAAA", "license": "MIT"}
}}`, base, integrity(tbs["/a.tgz"]), integrity(tbs["/b.tgz"]))
	if err := put(filepath.Join(here, "polyfill/package-lock.json"), lock); err != nil {
		return err
	}
	if err := put(filepath.Join(root, "LICENSE"), "MIT License\n\nCopyright tifa\n"); err != nil {
		return err
	}
	for _, a := range apps {
		app := filepath.Join(root, a.dir)
		bundle := "var " + a.name + "=1;\nexport{};\n"
		if err := put(filepath.Join(app, ".tifa/dist/ssr.js"), bundle); err != nil {
			return err
		}
		if err := put(filepath.Join(here, "bundles", a.name+".js"), bundle); err != nil {
			return err
		}
		if err := put(filepath.Join(app, "pnpm-lock.yaml"), "lock of "+a.name+"\n"); err != nil {
			return err
		}
		sources := `["../../src/app.tsx", "node_modules/react/index.js", "node_modules/react/other.js", "node_modules/@scope/pkg/lib/x.js", "node_modules/react-remove-scroll-bar/dist/y.js"]`
		if a.name == "bench" {
			sources = `["node_modules/react/index.js"]`
		}
		if err := put(filepath.Join(app, ".tifa/dist/ssr.js.map"), `{"sources": `+sources+`}`); err != nil {
			return err
		}
		nm := filepath.Join(app, "node_modules")
		for _, p := range []struct{ name, version, license, text string }{
			{"react", "19.0.0", "MIT", "MIT License\n\nCopyright a  \n"},
			{"@scope/pkg", "1.2.3", "ISC", "ISC License\r\nline two\r\n"},
		} {
			if err := put(filepath.Join(nm, p.name, "package.json"), fmt.Sprintf(`{"name":%q,"version":%q,"license":%q}`, p.name, p.version, p.license)); err != nil {
				return err
			}
			if err := put(filepath.Join(nm, p.name, "LICENSE.md"), p.text); err != nil {
				return err
			}
			if err := put(filepath.Join(nm, p.name, "README.md"), "not a notice\n"); err != nil {
				return err
			}
		}
		if err := put(filepath.Join(nm, "react-remove-scroll-bar/package.json"), `{"name":"react-remove-scroll-bar","version":"2.3.8","license":"MIT"}`); err != nil {
			return err
		}
	}
	return nil
}

func server() *httptest.Server {
	tbs := tarballs()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := tbs[r.URL.Path]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
}

func stubLicenseFetch(t *testing.T, srv *httptest.Server) {
	old := fetch
	t.Cleanup(func() { fetch = old })
	fetch = func(url string) ([]byte, error) {
		if strings.HasPrefix(url, "https://raw.githubusercontent.com/") {
			return []byte("MIT License\n\nCopyright scroll bar\n"), nil
		}
		return old(url)
	}
}

func TestGenerate(t *testing.T) {
	srv := server()
	defer srv.Close()
	stubLicenseFetch(t, srv)
	root, here := t.TempDir(), t.TempDir()
	if err := buildFixture(root, here, srv.URL); err != nil {
		t.Fatal(err)
	}
	out, err := generate(root, here)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| @fmt/polyfill-b | 2.1.0 | MIT | polyfill | [LICENSE](#text-2), [NOTICE.txt](#text-3) |",
		"| @scope/pkg | 1.2.3 | ISC | mui, shop | [LICENSE.md](#text-4) |",
		"| polyfill-a | 1.0.0 | MIT | polyfill | [LICENSE](#text-2) |",
		"| react | 19.0.0 | MIT | bench, mui, shop | [LICENSE.md](#text-2) |",
		"| react-remove-scroll-bar | 2.3.8 | MIT | mui, shop | [https://raw.githubusercontent.com/theKashey/react-remove-scroll-bar/8ca9ba5ea52de03308fe8ced94f7b159a44d28ff/LICENSE](#text-5) |",
		"tifa source license: [text 1](#text-1).",
		"## Text 4\n\n```text\nISC License\nline two\n```",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the notices lack %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "| esbuild |") || strings.Contains(out, "not at the top") || strings.Contains(out, "not a notice") {
		t.Errorf("the notices hold something they leave out:\n%s", out)
	}
}

// A bundle that is not the pinned one, and a tarball that is not the locked
// one, stop the generator.
func TestGenerateRefuses(t *testing.T) {
	srv := server()
	defer srv.Close()
	stubLicenseFetch(t, srv)
	root, here := t.TempDir(), t.TempDir()
	if err := buildFixture(root, here, srv.URL); err != nil {
		t.Fatal(err)
	}
	if err := put(filepath.Join(here, "bundles/mui.js"), "other\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(root, here); err == nil || !strings.Contains(err.Error(), "mui: bundle differs") {
		t.Errorf("a changed bundle: %v", err)
	}
	if err := buildFixture(root, here, srv.URL); err != nil {
		t.Fatal(err)
	}
	lock, _ := os.ReadFile(filepath.Join(here, "polyfill/package-lock.json"))
	bad := strings.Replace(string(lock), integrity(tarballs()["/a.tgz"]), "sha512-AAAA", 1)
	if err := put(filepath.Join(here, "polyfill/package-lock.json"), bad); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(root, here); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Errorf("a wrong integrity hash: %v", err)
	}
}
