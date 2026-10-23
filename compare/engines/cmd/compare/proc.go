// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Every engine process runs under a hard memory cap. GOMEMLIMIT is not
// enough: the QuickJS ports built by ccgo allocate outside the Go heap. On Linux the cap is a cgroup
// (systemd-run --user --scope -p MemoryMax), which counts all of it. On top
// of that, and as the only cap elsewhere, the driver reads the resident size
// of the process and kills it when it goes over.
var (
	flagMemCap    = flag.String("memcap", "4G", "memory cap per engine process for timed workloads and probes")
	flagMemCap262 = flag.String("memcap-test262", "2G", "memory cap per test262 process")
	flagCgroup    = flag.Bool("cgroup", true, "on Linux, also put each engine process in a systemd scope with MemoryMax")
	cgroupOK      bool
	cgroupChecked sync.Once
)

func parseSize(s string) int64 {
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	check(err)
	return n * mult
}

// useCgroup reports whether systemd-run can give a process a memory cap.
func useCgroup() bool {
	cgroupChecked.Do(func() {
		if runtime.GOOS != "linux" || !*flagCgroup {
			return
		}
		cgroupOK = exec.Command("systemd-run", "--user", "--scope", "-q", "-p", "MemoryMax=64M", "true").Run() == nil
	})
	return cgroupOK
}

// capDesc says how processes are capped, for the results.
func capDesc() string {
	how := "resident size checked by the driver"
	if useCgroup() {
		how = "cgroup MemoryMax (systemd-run --user --scope), swap off, and resident size checked by the driver"
	}
	return fmt.Sprintf("%s for timed workloads and probes, %s for test262: %s", *flagMemCap, *flagMemCap262, how)
}

// argv puts the cgroup cap and the CPU pinning in front of the command.
func argv(limit int64, cpu string, nice int, bin string, args []string) []string {
	var a []string
	if useCgroup() {
		a = append(a, "systemd-run", "--user", "--scope", "-q",
			"-p", "MemoryMax="+strconv.FormatInt(limit, 10), "-p", "MemorySwapMax=0")
	}
	if cpu != "" {
		a = append(a, "taskset", "-c", cpu)
	}
	if nice != 0 {
		a = append(a, "nice", "-n", strconv.Itoa(nice))
	}
	return append(append(a, bin), args...)
}

// guard watches a started process. kill ends it and remembers why; the
// watcher kills it with reason "memory" when its resident size goes over
// the limit.
type guard struct {
	cmd    *exec.Cmd
	limit  int64
	mu     sync.Mutex
	reason string
	done   chan struct{}
}

func newGuard(cmd *exec.Cmd, limit int64) *guard {
	g := &guard{cmd: cmd, limit: limit, done: make(chan struct{})}
	every := 50 * time.Millisecond
	if runtime.GOOS != "linux" {
		every = 250 * time.Millisecond // ps is slower than /proc
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-g.done:
				return
			case <-t.C:
				if rssOf(cmd.Process.Pid) > limit {
					g.kill("memory")
					return
				}
			}
		}
	}()
	return g
}

func (g *guard) kill(reason string) {
	g.mu.Lock()
	if g.reason == "" {
		g.reason = reason
	}
	g.mu.Unlock()
	g.cmd.Process.Kill()
}

// why stops the watcher after Wait and says why the process ended early:
// "" (it exited by itself), "timeout", or "memory". A SIGKILL the driver
// did not send comes from the cgroup cap.
func (g *guard) why() string {
	close(g.done)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.reason != "" {
		return g.reason
	}
	if st := g.cmd.ProcessState; st != nil {
		if ws, ok := st.Sys().(syscall.WaitStatus); ok && ws.Signaled() && ws.Signal() == syscall.SIGKILL {
			return "memory"
		}
	}
	return ""
}

// rssOf returns the resident size of pid in bytes, 0 when it is gone.
func rssOf(pid int) int64 {
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
		if err != nil {
			return 0
		}
		f := strings.Fields(string(b))
		if len(f) < 2 {
			return 0
		}
		n, _ := strconv.ParseInt(f[1], 10, 64)
		return n * int64(os.Getpagesize())
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return n << 10
}

func outOfMemory(limit int64) string {
	return fmt.Sprintf("out of memory: over the %d MiB cap", limit>>20)
}
