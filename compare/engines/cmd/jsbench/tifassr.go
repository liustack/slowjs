// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liustack/slowjs/compare/engines/engine"
)

// ---- a tifa SSR bundle ----
//
// A tifa SSR bundle defines globalThis.startRender(json). It receives the
// JSON of the render input and leaves globalThis.renderResult as a string,
// after the microtasks it queued have run. tifa registers Intl and nothing
// else on the VM. runTifaSSR loads such a bundle once, then renders each
// input warm times and n timed times in the same VM.

type tifaPage struct {
	Input  string  `json:"input"`
	Probe  string  `json:"probe,omitempty"`     // -probe after the first render
	ProbeN string  `json:"probe_end,omitempty"` // -probe after the timed renders
	OK     bool    `json:"ok"`
	Err    string  `json:"err,omitempty"`
	Bytes  int     `json:"html_bytes"`
	Sum    string  `json:"html_sum"`
	Render stats   `json:"render"`
	CPUMS  float64 `json:"cpu_ms_per_op"`
}

type tifaResult struct {
	PreludeMS []float64  `json:"prelude_ms,omitempty"` // load time of each -prelude script
	LoadMS    float64    `json:"load_ms"`
	Err       string     `json:"err,omitempty"`
	Pages     []tifaPage `json:"pages"`
	Mem       *memResult `json:"mem,omitempty"`
}

// tifaOpts are the tifa-ssr flags besides the bundle and inputs.
type tifaOpts struct {
	intl     bool     // register the engine's own Intl where it is opt-in
	preludes []string // scripts run before the bundle, in order
	probe    string   // an expression whose String() is recorded per page
	tag      string   // added to the names of the HTML files
	mem      int      // VMs kept alive to measure memory per VM, 0 for none
}

// tifaVM makes a VM with the preludes and the bundle loaded and the glue
// installed.
func tifaVM(e *engine.Engine, bundle string, o tifaOpts, r *tifaResult) (engine.VM, error) {
	vm, err := e.New(engine.Options{Intl: o.intl, Clock: true})
	if err != nil {
		return nil, err
	}
	for _, p := range o.preludes {
		t := time.Now()
		if err := load(vm, filepath.Base(p), readFile(p)); err != nil {
			vm.Close()
			return nil, fmt.Errorf("prelude %s: %v", filepath.Base(p), errString(err))
		}
		if r != nil {
			r.PreludeMS = append(r.PreludeMS, ms(time.Since(t)))
		}
	}
	t := time.Now()
	if err := load(vm, filepath.Base(bundle), readFile(bundle)); err != nil {
		vm.Close()
		return nil, fmt.Errorf("load: %v", errString(err))
	}
	if r != nil {
		r.LoadMS = ms(time.Since(t))
	}
	if _, err := vm.Eval("glue.js", tifaGlue); err != nil {
		vm.Close()
		return nil, fmt.Errorf("glue: %v", errString(err))
	}
	return vm, nil
}

func tifaRender(vm engine.VM) (string, error) {
	if _, err := vm.Call("__go"); err != nil {
		return "", err
	}
	if err := vm.RunJobs(); err != nil {
		return "", err
	}
	return vm.Call("__res")
}

func tifaInput(vm engine.VM, in string) error {
	lit, _ := json.Marshal(readFile(in))
	_, err := vm.Eval("input.js", "globalThis.__in = "+string(lit))
	return err
}

func runTifaSSR(e *engine.Engine, bundle string, inputs []string, outDir string, warm, n int, o tifaOpts) (r tifaResult) {
	vm, err := tifaVM(e, bundle, o, &r)
	if err != nil {
		r.Err = errString(err)
		return
	}
	defer vm.Close()
	render := func() (string, error) { return tifaRender(vm) }
	probe := func() string {
		if o.probe == "" {
			return ""
		}
		s, err := vm.Eval("probe.js", "String("+o.probe+")")
		if err != nil {
			return "probe: " + errString(err)
		}
		return s
	}
	for _, in := range inputs {
		p := tifaPage{Input: filepath.Base(in)}
		if err := tifaInput(vm, in); err != nil {
			p.Err = errString(err)
			r.Pages = append(r.Pages, p)
			continue
		}
		html, err := render()
		p.Probe = probe()
		if err != nil {
			p.Err = errString(err)
			r.Pages = append(r.Pages, p)
			continue
		}
		p.Bytes, p.Sum = len(html), sum(html)
		if outDir != "" {
			os.WriteFile(filepath.Join(outDir, e.Name+o.tag+"-"+strings.TrimSuffix(p.Input, ".json")+".html"), []byte(html), 0o644)
		}
		for i := 0; i < warm && p.Err == ""; i++ {
			if _, err := render(); err != nil {
				p.Err = errString(err)
			}
		}
		d := make([]time.Duration, 0, n)
		c0 := cpuTime()
		for i := 0; i < n && p.Err == ""; i++ {
			t := time.Now()
			h, err := render()
			d = append(d, time.Since(t))
			if err != nil {
				p.Err = errString(err)
			} else if len(h) != len(html) {
				p.Err = "output changed between renders"
			}
		}
		if p.Err == "" && n > 0 {
			p.CPUMS = ms(cpuTime()-c0) / float64(n)
			p.OK, p.Render = true, summarize(d)
			p.ProbeN = probe()
		}
		r.Pages = append(r.Pages, p)
	}
	if o.mem > 0 && len(inputs) > 0 {
		r.Mem = tifaMem(e, bundle, inputs[0], o)
	}
	return
}

// tifaMem keeps o.mem VMs alive, each with everything loaded and one page
// rendered, and reports the growth of the process per VM.
func tifaMem(e *engine.Engine, bundle, input string, o tifaOpts) *memResult {
	m := &memResult{K: o.mem, Loaded: "source"}
	one := func() (engine.VM, error) {
		vm, err := tifaVM(e, bundle, o, nil)
		if err != nil {
			return nil, err
		}
		if err := tifaInput(vm, input); err == nil {
			_, err = tifaRender(vm)
		}
		return vm, err
	}
	h0 := heap()
	m.RSSBase = rss()
	vms := make([]engine.VM, 0, o.mem)
	defer func() {
		for _, vm := range vms {
			vm.Close()
		}
	}()
	for i := 0; i < o.mem; i++ {
		vm, err := one()
		if err != nil {
			m.Err = errString(err)
			return m
		}
		vms = append(vms, vm)
	}
	m.RSSPerVM = (rss() - m.RSSBase) / float64(o.mem)
	m.GoHeapPerVM = (heap() - h0) / float64(o.mem)
	m.OK = true
	return m
}

const tifaGlue = `globalThis.__go = function () { globalThis.renderResult = undefined; startRender(globalThis.__in) };
globalThis.__res = function () { var s = globalThis.renderResult; globalThis.renderResult = undefined; return typeof s === "string" ? s : "not a string: " + String(s) }`
