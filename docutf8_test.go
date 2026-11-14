// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

// notUTF8 returns the files of files whose text read gives is not valid UTF-8.
// A file that is gone from the working tree is left out.
func notUTF8(files []string, read func(string) ([]byte, error)) ([]string, error) {
	var bad []string
	for _, f := range files {
		b, err := read(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(b) {
			bad = append(bad, f)
		}
	}
	return bad, nil
}

func TestNotUTF8(t *testing.T) {
	text := map[string]string{
		"a.md":    "发布版本：v0.30.5。\n",
		"b.md":    "发布版本：\x80\x82\n",
		"gone.md": "",
	}
	read := func(f string) ([]byte, error) {
		if f == "gone.md" {
			return nil, fs.ErrNotExist
		}
		return []byte(text[f]), nil
	}
	got, err := notUTF8([]string{"a.md", "b.md", "gone.md"}, read)
	if err != nil || len(got) != 1 || got[0] != "b.md" {
		t.Errorf("notUTF8 = %v, %v, want [b.md]", got, err)
	}
}

// TestDocsUTF8 checks that every tracked .md file is valid UTF-8, so that a
// replacement that cuts a character in two cannot be committed.
func TestDocsUTF8(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "-z", "--", "*.md").Output()
	if err != nil {
		t.Skip("not a git checkout:", err)
	}
	files := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	if len(files) == 0 || files[0] == "" {
		t.Fatal("git ls-files lists no .md file")
	}
	bad, err := notUTF8(files, os.ReadFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Errorf("not valid UTF-8: %v", bad)
	}
}
