//go:build testhooks

package cengine

import (
	"os"
	"strings"
	"testing"
)

// ZZ_CASES="name|setup|call;name|setup|call" ZZ_FILE overrides
func TestZZMeasure(t *testing.T) {
	needPolls(t)
	src := os.Getenv("ZZ_CASES")
	if src == "" {
		t.Skip("no ZZ_CASES")
	}
	for _, c := range strings.Split(src, "\n") {
		if strings.TrimSpace(c) == "" {
			continue
		}
		parts := strings.SplitN(c, "|", 3)
		t.Run(parts[0], func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(parts[1], "setup.js"); err != nil {
				t.Fatal(err)
			}
			v.SetGCThreshold(^uintptr(0))
			v.RunGC()
			var polls [3]int64
			for i := range polls {
				before := h.Polls()
				if _, err := v.Eval(parts[2]+"; 0", "call.js"); err != nil {
					t.Fatal(err)
				}
				polls[i] = h.Polls() - before
			}
			t.Logf("RESULT %s: %v polls", parts[0], polls)
		})
	}
}
