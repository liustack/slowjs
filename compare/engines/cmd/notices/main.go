// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command notices generates tifa/THIRD_PARTY_NOTICES.md from matching tifa
// builds and npm metadata.
//
//	go run ./cmd/notices /path/to/tifa [tifa folder of the comparison]
//
// Run it in compare/engines. It reads installed packages and source maps
// without building or installing. It fetches the locked FormatJS tarballs
// only for their metadata and license files.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/liustack/slowjs/compare/engines/internal/pytext"
)

var apps = []struct{ name, dir string }{{"bench", "test/apps/bench"}, {"mui", "test/apps/mui"}, {"shop", "examples/shop"}}

// sourceLicenses are the licenses that an npm tarball omits. They are kept
// from the upstream project at an immutable commit, and the notice discloses
// that separate source.
var sourceLicenses = map[[2]string]string{
	{"react-remove-scroll-bar", "2.3.8"}: "https://raw.githubusercontent.com/theKashey/react-remove-scroll-bar/" +
		"8ca9ba5ea52de03308fe8ced94f7b159a44d28ff/LICENSE",
}

var noticeName = regexp.MustCompile(`(?i)^(licen[cs]e|notice|copying|copyright)`)

// fetch gets a URL.
var fetch = func(url string) ([]byte, error) {
	c := http.Client{Timeout: 60 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// text is the name of a file or tarball member with its text.
type text struct{ name, body string }

type pkg struct {
	license string
	texts   []text
	apps    map[string]bool
}

type meta struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
}

// readText reads a file as text with its line ends as \n.
func readText(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("%s is not UTF-8", p)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n"), nil
}

type generator struct {
	packages map[[2]string]*pkg
}

func (g *generator) add(m meta, texts []text, app string) error {
	key := [2]string{m.Name, m.Version}
	if len(texts) == 0 {
		url, ok := sourceLicenses[key]
		if !ok {
			return fmt.Errorf("%s %s has no license text and no source for one", m.Name, m.Version)
		}
		b, err := fetch(url)
		if err != nil {
			return err
		}
		texts = []text{{url, string(b)}}
	}
	e, ok := g.packages[key]
	if !ok {
		e = &pkg{license: m.License, texts: texts, apps: map[string]bool{}}
		g.packages[key] = e
	}
	if e.license != m.License {
		return fmt.Errorf("%s %s: the license differs between %s and an earlier use", m.Name, m.Version, app)
	}
	if len(e.texts) != len(texts) {
		return fmt.Errorf("%s %s: the license texts differ in number", m.Name, m.Version)
	}
	for i := range texts {
		if e.texts[i].body != texts[i].body {
			return fmt.Errorf("%s %s: the license text %d differs", m.Name, m.Version, i)
		}
	}
	e.apps[app] = true
	return nil
}

func hasher(algorithm string) (hash.Hash, error) {
	switch algorithm {
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha384":
		return sha512.New384(), nil
	case "sha512":
		return sha512.New(), nil
	}
	return nil, fmt.Errorf("unknown integrity algorithm %q", algorithm)
}

// generate makes the notices from the tifa checkout root and the pinned files
// of the comparison in here.
func generate(root, here string) (string, error) {
	g := &generator{packages: map[[2]string]*pkg{}}
	type row struct{ app, bundle, srcmap, lock string }
	var provenance []row
	for _, a := range apps {
		base := filepath.Join(root, a.dir)
		bundle, err := os.ReadFile(filepath.Join(base, ".tifa/dist/ssr.js"))
		if err != nil {
			return "", err
		}
		pinned, err := os.ReadFile(filepath.Join(here, "bundles", a.name+".js"))
		if err != nil {
			return "", err
		}
		if !bytes.Equal(bundle, pinned) {
			return "", fmt.Errorf("%s: bundle differs", a.name)
		}
		srcmap, err := os.ReadFile(filepath.Join(base, ".tifa/dist/ssr.js.map"))
		if err != nil {
			return "", err
		}
		lock, err := os.ReadFile(filepath.Join(base, "pnpm-lock.yaml"))
		if err != nil {
			return "", err
		}
		provenance = append(provenance, row{a.name, sha(bundle), sha(srcmap), sha(lock)})
		var sm struct {
			Sources []string `json:"sources"`
		}
		if err := json.Unmarshal(srcmap, &sm); err != nil {
			return "", err
		}
		seen := map[string]bool{}
		for _, source := range sm.Sources {
			i := strings.LastIndex(source, "node_modules/")
			if i < 0 {
				continue
			}
			prefix, suffix := source[:i], source[i+len("node_modules/"):]
			parts := strings.Split(suffix, "/")
			name := parts[0]
			if strings.HasPrefix(suffix, "@") && len(parts) > 1 {
				name = parts[0] + "/" + parts[1]
			}
			// Not cleaned: the path goes through links, and ".." must follow them.
			dir := base + "/" + prefix + "node_modules/" + name
			if seen[dir] {
				continue
			}
			seen[dir] = true
			metaText, err := os.ReadFile(dir + "/package.json")
			if err != nil {
				return "", err
			}
			var m meta
			if err := json.Unmarshal(metaText, &m); err != nil {
				return "", fmt.Errorf("%s/package.json: %v", dir, err)
			}
			if m.Name != name {
				return "", fmt.Errorf("%s: the package is %s", source, m.Name)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", err
			}
			var texts []text
			for _, e := range entries { // sorted by name
				st, err := os.Stat(dir + "/" + e.Name())
				if err != nil || !st.Mode().IsRegular() || !noticeName.MatchString(e.Name()) {
					continue
				}
				body, err := readText(dir + "/" + e.Name())
				if err != nil {
					return "", err
				}
				texts = append(texts, text{e.Name(), body})
			}
			if err := g.add(m, texts, a.name); err != nil {
				return "", err
			}
		}
	}

	lockBytes, err := os.ReadFile(filepath.Join(here, "polyfill/package-lock.json"))
	if err != nil {
		return "", err
	}
	var lockFile struct {
		Packages map[string]struct {
			Version      string            `json:"version"`
			Resolved     string            `json:"resolved"`
			Integrity    string            `json:"integrity"`
			License      string            `json:"license"`
			Dependencies map[string]string `json:"dependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lockBytes, &lockFile); err != nil {
		return "", err
	}
	// Follow the runtime dependency closure, excluding esbuild and its platform
	// executables. They build the script but do not appear in it.
	var pending []string
	for n := range lockFile.Packages[""].Dependencies {
		if n != "esbuild" {
			pending = append(pending, n)
		}
	}
	sort.Strings(pending)
	seen := map[string]bool{}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[name] {
			continue
		}
		seen[name] = true
		item, ok := lockFile.Packages["node_modules/"+name]
		if !ok {
			return "", fmt.Errorf("%s is not in the lockfile", name)
		}
		data, err := fetch(item.Resolved)
		if err != nil {
			return "", err
		}
		algorithm, expected, _ := strings.Cut(item.Integrity, "-")
		h, err := hasher(algorithm)
		if err != nil {
			return "", fmt.Errorf("%s: %v", name, err)
		}
		h.Write(data)
		if base64.StdEncoding.EncodeToString(h.Sum(nil)) != expected {
			return "", fmt.Errorf("%s: the tarball does not match its integrity hash", name)
		}
		m, texts, err := readTarball(data)
		if err != nil {
			return "", fmt.Errorf("%s: %v", name, err)
		}
		if m.Name != name || m.Version != item.Version || m.License != item.License {
			return "", fmt.Errorf("%s: the tarball is %s %s %s, the lockfile has %s %s", name, m.Name, m.Version, m.License, item.Version, item.License)
		}
		if err := g.add(m, texts, "polyfill"); err != nil {
			return "", err
		}
		var deps []string
		for d := range item.Dependencies {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		pending = append(pending, deps...)
	}

	lines := []string{"<!-- Copyright (c) 2026 The slowjs Authors. -->",
		"<!-- SPDX-License-Identifier: MIT -->",
		"<!-- The generator and inventory are MIT under LICENSE-MIT at the repository root. -->",
		"<!-- The copied third-party notices below retain their own terms. -->", "",
		"# tifa bundle third-party notices", "",
		"Generated by `go run ./cmd/notices /path/to/tifa` in compare/engines.",
		"The three source bundles must match the committed files byte for byte.",
		"Their source maps identify the npm packages. Names, versions and licenses",
		"come from those installed packages, with their license and notice files copied below.",
		"Identical texts share a section. Line-end whitespace is omitted.", "",
		"The polyfill rows cover the runtime dependency closure in",
		"`polyfill/package-lock.json`. Metadata and notices come from the tarballs",
		"at its resolved URLs, checked against its integrity hashes. esbuild is a",
		"build tool, not bundled code. The polyfill itself is not committed.", "",
		"The tifa app and runtime code is MIT. Its license is included below.",
		"react-remove-scroll-bar omits a license file from its npm package.",
		"Its metadata declares MIT. The text below comes from its upstream project",
		"at commit `8ca9ba5ea52de03308fe8ced94f7b159a44d28ff`, where LICENSE was added.", "",
		"## Source fingerprints", "",
		"| Bundle | SHA256 | Source map SHA256 | App lockfile SHA256 |",
		"| --- | --- | --- | --- |"}
	for _, r := range provenance {
		lines = append(lines, "| "+strings.Join([]string{r.app, r.bundle, r.srcmap, r.lock}, " | ")+" |")
	}
	lines = append(lines, "", "Polyfill lockfile SHA256: `"+sha(lockBytes)+"`.", "", "## Packages", "",
		"| Package | Version | License | Bundles | Texts |", "| --- | --- | --- | --- | --- |")
	var notices []string // the distinct texts, numbered from 1 in order of first use
	reference := func(body string) int {
		var cl []string
		for _, l := range pytext.SplitLines(body) {
			cl = append(cl, strings.TrimRightFunc(l, pytext.IsSpace))
		}
		cleaned := strings.TrimFunc(strings.Join(cl, "\n"), pytext.IsSpace) + "\n"
		if i := slices.Index(notices, cleaned); i >= 0 {
			return i + 1
		}
		notices = append(notices, cleaned)
		return len(notices)
	}
	licenseText, err := readText(filepath.Join(root, "LICENSE"))
	if err != nil {
		return "", err
	}
	tifaRef := reference(licenseText)
	keys := make([][2]string, 0, len(g.packages))
	for k := range g.packages {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]string) int {
		if c := strings.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return strings.Compare(a[1], b[1])
	})
	for _, k := range keys {
		p := g.packages[k]
		var refs []string
		for _, t := range p.texts {
			refs = append(refs, fmt.Sprintf("[%s](#text-%d)", t.name, reference(t.body)))
		}
		var as []string
		for a := range p.apps {
			as = append(as, a)
		}
		sort.Strings(as)
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %s |", k[0], k[1], p.license, strings.Join(as, ", "), strings.Join(refs, ", ")))
	}
	lines = append(lines, "", fmt.Sprintf("tifa source license: [text %d](#text-%d).", tifaRef, tifaRef))
	for i, body := range notices {
		lines = append(lines, "", fmt.Sprintf("## Text %d", i+1), "", "```text", strings.TrimRightFunc(body, pytext.IsSpace), "```")
	}
	return strings.TrimRightFunc(strings.Join(lines, "\n"), pytext.IsSpace) + "\n", nil
}

// readTarball reads package/package.json and the license and notice files
// that sit directly in package/ of an npm tarball.
func readTarball(data []byte) (meta, []text, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return meta{}, nil, err
	}
	tr := tar.NewReader(zr)
	files := map[string][]byte{}
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return meta{}, nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return meta{}, nil, err
		}
		files[h.Name] = b
		names = append(names, h.Name)
	}
	var m meta
	pj, ok := files["package/package.json"]
	if !ok {
		return meta{}, nil, fmt.Errorf("no package/package.json")
	}
	if err := json.Unmarshal(pj, &m); err != nil {
		return meta{}, nil, err
	}
	sort.Strings(names)
	var texts []text
	for _, n := range names {
		parts := strings.Split(path.Clean(n), "/")
		if len(parts) == 2 && noticeName.MatchString(parts[1]) {
			if !utf8.Valid(files[n]) {
				return meta{}, nil, fmt.Errorf("%s is not UTF-8", n)
			}
			texts = append(texts, text{strings.TrimPrefix(n, "package/"), string(files[n])})
		}
	}
	return m, texts, nil
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: notices /path/to/tifa [folder of the pinned files, default tifa]")
		os.Exit(2)
	}
	root, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
	here := "tifa"
	if len(os.Args) == 3 {
		here = os.Args[2]
	}
	out, err := generate(root, here)
	if err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(here, "THIRD_PARTY_NOTICES.md"), []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}
