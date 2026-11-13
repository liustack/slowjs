// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/whatwgurl"
	"github.com/liustack/quickjs/internal/whatwgurl/idna"
)

// The room a Builtin checks before it reads its strings, roomPerUnit bytes for
// each UTF-16 unit and roomFixed, is at least what Go then allocates for the
// call: otherwise Go would take memory the VM's limit did not let it have.

// quiet makes the value of js nothing: converting a URL or a long string to a
// Go value for Eval to return takes time of its own, and calls toJSON.
func quiet(js string) string { return "(function () { " + js + "; })(); 1" }

func isInterrupted(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && e.Uncatchable && e.Interrupted
}

func isOOM(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && e.Uncatchable && e.OutOfMemory
}

// catchable reports whether err is an error a script can catch, which a call
// may end with when it is given what is not a URL or a field too long.
func catchable(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && !e.Uncatchable
}

// shapes are the inputs that make the work of Go long: the text T, of N
// units, and the whole URL I that holds it, both at most MaxUnits.
var shapes = map[string][2]string{
	"path":            {`"a".repeat(N)`, `"http://h/" + T`},
	"slashes":         {`"/".repeat(N)`, `"http://h/" + T`},
	"segments":        {`"a/".repeat(N / 2)`, `"http://h/" + T`},
	"dotdot":          {`"../".repeat(N / 3)`, `"http://h/" + T`},
	"backslashes":     {`"\\".repeat(N)`, `"http://h/" + T`},
	"query":           {`"a".repeat(N)`, `"http://h/?" + T`},
	"fragment":        {`"a".repeat(N)`, `"http://h/#" + T`},
	"opaque":          {`"a".repeat(N)`, `"x:" + T`},
	"opaque spaces":   {`" ".repeat(N)`, `"x:a" + T + "?b"`},
	"userinfo":        {`":".repeat(N)`, `"http://" + T + "@h/"`},
	"userinfo at":     {`"a@".repeat(N / 2)`, `"http://" + T + "h/"`},
	"host":            {`"a".repeat(N)`, `"http://" + T + "/"`},
	"host dots":       {`"a.".repeat(N / 2)`, `"http://" + T + "/"`},
	"host wide":       {`"\u00e9".repeat(N)`, `"http://" + T + "/"`},
	"host idna":       {`"\u00e9".repeat(1000)`, `"http://" + T + "/"`},
	"host distinct":   {`Array.from({length: 1024}, (_, i) => String.fromCharCode(0x4e00 + i)).join("")`, `"http://" + T + "/"`},
	"host marks":      {`"\u0315\u0300".repeat(511)`, `"http://a" + T + "/"`},
	"host percent":    {`"%C3%A9".repeat(N / 6)`, `"http://" + T + "/"`},
	"host punycode":   {`"xn--".repeat(N / 4)`, `"http://" + T + "/"`},
	"host opaque":     {`"\u00e9".repeat(N)`, `"x://" + T + "/"`},
	"ipv6":            {`"1:".repeat(N / 2)`, `"http://[" + T + "]/"`},
	"ipv4 like":       {`"1.".repeat(N / 2)`, `"http://" + T + "/"`},
	"tabs":            {`"a\t".repeat(N / 2)`, `"http://h/" + T`},
	"spaces":          {`" ".repeat(N / 2)`, `T + "http://h/" + T`},
	"percent":         {`"%41".repeat(N / 3)`, `"http://h/?" + T`},
	"percent invalid": {`"%80".repeat(N / 3)`, `"http://h/?" + T`},
	"plus and equals": {`"+=".repeat(N / 2)`, `"http://h/?" + T`},
	"ampersands":      {`"a&".repeat(N / 2)`, `"http://h/?" + T`},
	"pairs":           {`"a=b&".repeat(N / 4)`, `"http://h/?" + T`},
	"wide":            {`"\u4e00".repeat(N / 2)`, `"http://h/?" + T + "#" + T`},
	"path wide":       {`"\u4e00".repeat(N)`, `"http://h/" + T`},
	"opaque wide":     {`"\u4e00".repeat(N)`, `"x:" + T`},
	"userinfo wide":   {`"\u4e00".repeat(N)`, `"http://" + T + "@h/"`},
	"segments wide":   {`"\u4e00/".repeat(N / 2)`, `"http://h/" + T`},
	"surrogates":      {`"\ud83d\ude00".repeat(N / 2)`, `"http://h/" + T`},
	"lone surrogates": {`"\ud83d".repeat(N)`, `"http://h/" + T`},
}

// The hosts of the code points whose IDNA grows the most for each unit they
// take, as the tables have them (idna.Widest): one that maps to the most
// units, past the limit of a domain, at it, and as many as map to no more than
// it, and one that decomposes to the most, at the limit.
func init() {
	m, mn, d, _ := idna.Widest()
	h := whatwgurl.MaxDomainUnits
	cp := func(r rune, n int) string {
		return "String.fromCodePoint(" + strconv.Itoa(int(r)) + ").repeat(" + strconv.Itoa(n) + ")"
	}
	shapes["host widest mapping"] = [2]string{cp(m, h) + ".repeat(N / " + strconv.Itoa(h) + ")", `"http://" + T + "/"`}
	shapes["host widest mapping at the limit"] = [2]string{cp(m, h), `"http://" + T + "/"`}
	shapes["host widest mapping, mapped to the limit"] = [2]string{cp(m, h/mn), `"http://" + T + "/"`}
	shapes["host widest mapping, labels"] = [2]string{`(` + cp(m, 3) + ` + ".").repeat(` + strconv.Itoa(h/(3*mn+1)) + `)`, `"http://" + T + "/"`}
	shapes["host widest decomposition at the limit"] = [2]string{cp(d, h), `"http://" + T + "/"`}
	shapes["host widest decomposition, labels"] = [2]string{`(` + cp(d, 60) + ` + ".").repeat(16)`, `"http://" + T + "/"`}
	// A label of Punycode, in a domain that is not ASCII, that decodes to a
	// character of two units for each of its letters.
	shapes["host punycode decoded"] = [2]string{`"é.xn--e28h" + "a".repeat(` + strconv.Itoa(h-11) + `)`, `"http://" + T + "/"`}
	shapes["host punycode decoded, labels"] = [2]string{`"é" + ".xn--e28haaaa".repeat(` + strconv.Itoa((h-1)/13) + `)`, `"http://" + T + "/"`}
}

// entries are the calls that take the text T or the URL I, and the objects
// made before from them.
var entries = []string{
	`new URL(I)`,
	`new URL(I, "http://b/")`,
	`new URL("x", I)`,
	`new URL(I, I)`,
	`URL.canParse(I)`,
	`URL.parse(I)`,
	`new URL("http://h/?a#b").href = I`,
	`new URL("http://h/?a#b").protocol = T`,
	`new URL("http://h/?a#b").username = T`,
	`new URL("http://h/?a#b").password = T`,
	`new URL("http://h/?a#b").host = T`,
	`new URL("http://h/?a#b").hostname = T`,
	`new URL("http://h/?a#b").port = T`,
	`new URL("http://h/?a#b").pathname = T`,
	`new URL("http://h/?a#b").search = T`,
	`new URL("http://h/?a#b").hash = T`,
	`bigURL.search = "x"`,
	`bigURL.hash = T`,
	`bigURL.pathname = "/x"`,
	`bigURL.searchParams`,
	`bigURL.searchParams.append("k", "v")`,
	`new URL("http://h/?a=b").searchParams.append(T, T)`,
	`new URL("http://h/?a=b").searchParams.set("a", T)`,
	`new URLSearchParams(T)`,
	`new URLSearchParams(I)`,
	`new URLSearchParams([[T, T]]).toString()`,
	`new URLSearchParams({[T]: T}).toString()`,
	`new URLSearchParams(bigP)`,
	`bigP.toString()`,
	`bigP.sort()`,
	`bigP.set("a", "b")`,
}

const setup = `
	var N = SIZE;
	var T = SHAPE;
	var I = IN;
	var bigURL, bigP;
	try { bigURL = new URL(I); } catch (e) { bigURL = new URL("http://h/?a#b"); }
	try { bigP = new URLSearchParams(T); } catch (e) { bigP = new URLSearchParams("a=b"); }
	"ok"`

// shapeVM is a VM with the shape name set up, T of n units.
func shapeVM(t testing.TB, name string, n int) *quickjs.VM {
	vm := newVM(t)
	sh := shapes[name]
	evalString(t, vm, strings.NewReplacer("SHAPE", sh[0], "IN", sh[1], "SIZE", strconv.Itoa(n)).Replace(setup))
	return vm
}

// measureCalls runs js on vm and returns, for each call of a Builtin, what Go
// allocated in it and the room admit checked for it.
func measureCalls(t testing.TB, vm *quickjs.VM, js string) (grown, room []int64, err error) {
	var before runtime.MemStats
	need := int64(-1)
	callHook = func(enter bool) {
		if enter {
			need = -1
			runtime.ReadMemStats(&before)
			return
		}
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		grown = append(grown, int64(after.TotalAlloc-before.TotalAlloc))
		room = append(room, need)
	}
	admitHook = func(n int64) { need = n }
	defer func() { callHook, admitHook = nil, nil }()
	_, err = vm.Eval(quiet(js), quickjs.EvalGlobal)
	return
}

// wantRoomCovers fails t when a call of a Builtin that js makes allocates more
// in Go than the room it checked. It returns the largest share of the room a
// call used.
func wantRoomCovers(t testing.TB, vm *quickjs.VM, js string) (share float64) {
	t.Helper()
	grown, room, err := measureCalls(t, vm, js)
	if err != nil && !catchable(err) {
		t.Errorf("%s: %v", js, err)
		return 0
	}
	for i := range grown {
		if room[i] < 0 {
			// A call that ended before it read: a string past the limit.
			if grown[i] > 4<<10 {
				t.Errorf("%s: call %d read nothing and allocated %d bytes", js, i, grown[i])
			}
			continue
		}
		if grown[i] > room[i] {
			t.Errorf("%s: call %d allocated %d bytes in Go, more than the %d of room it checked", js, i, grown[i], room[i])
		}
		share = max(share, float64(grown[i])/float64(room[i]))
	}
	return share
}

// TestRoomCoversGo measures what Go allocates in every call of a Builtin, for
// every entry and every shape of input up to the limit, against the room the
// call checked: roomPerUnit and roomFixed may not be below it.
func TestRoomCoversGo(t *testing.T) {
	worst, at := 0.0, ""
	for name := range shapes {
		t.Run(name, func(t *testing.T) {
			vm := shapeVM(t, name, whatwgurl.MaxUnits-32)
			for _, js := range entries {
				if s := wantRoomCovers(t, vm, js); s > worst {
					worst, at = s, name+": "+js
				}
			}
		})
	}
	t.Logf("the most a call took is %.0f%% of its room, in %s", 100*worst, at)
}

// FuzzRoom is TestRoomCoversGo for a text made of a prefix, a unit repeated
// count times, or as many times as fit in MaxUnits when count is 0, and a
// suffix, given to every entry. The seeds have the code points the tables of
// IDNA grow the most (idna.Widest) in a host, as many as make a domain of the
// limit once mapped, and past it.
func FuzzRoom(f *testing.F) {
	for _, s := range [][3]string{
		{"http://h/", "a", ""}, {"http://h/?", "%80", ""}, {"http://", "é", "/"}, {"x:", "一", ""},
		{"http://h/?", "+=", ""}, {"", " ", "http://h/"}, {"http://", "̀̕", "/"}, {"http://h/", "\U0001F600", ""},
	} {
		f.Add(s[0], s[1], s[2], uint16(0))
	}
	m, mn, d, _ := idna.Widest()
	h := whatwgurl.MaxDomainUnits
	f.Add("http://", string(m), "/", uint16(h/mn))
	f.Add("http://", string(m), "/", uint16(h/mn+1))
	f.Add("http://a.", string(m)+"."+string(d), "/", uint16(h/(mn+2)))
	f.Add("http://", string(d), "/", uint16(h))
	f.Add("http://é.xn--e28h", "a", "/", uint16(h-11))
	f.Fuzz(func(t *testing.T, prefix, unit, suffix string, count uint16) {
		if unit == "" || len(prefix)+len(suffix) > whatwgurl.MaxUnits {
			return
		}
		vm := newVM(t)
		setGlobal(t, vm, "P", prefix)
		setGlobal(t, vm, "U", unit)
		setGlobal(t, vm, "S", suffix)
		if evalString(t, vm, strings.NewReplacer("MAX", strconv.Itoa(whatwgurl.MaxUnits), "COUNT", strconv.Itoa(int(count))).Replace(`
			var room = MAX - P.length - S.length, n = Math.floor(room / U.length);
			if (COUNT > 0 && COUNT < n) { n = COUNT; }
			var T = P + U.repeat(n) + S, I = T;
			var bigURL, bigP;
			try { bigURL = new URL(I); } catch (e) { bigURL = new URL("http://h/?a#b"); }
			try { bigP = new URLSearchParams(T); } catch (e) { bigP = new URLSearchParams("a=b"); }
			n > 0 ? "ok" : "no room"`)) != "ok" {
			return
		}
		for _, js := range entries {
			wantRoomCovers(t, vm, js)
		}
	})
}

// TestFirstNormalizationCounted checks that the tables of the first
// normalization of a process (NFC, and the mapping of IDNA) are built when a
// VM is registered and not in the first call that needs them, which would
// allocate them past the room the call checked (the first finding of the third
// review). It runs in a new process, where nothing was normalized before.
func TestFirstNormalizationCounted(t *testing.T) {
	if os.Getenv("URL_FIRST_NORMALIZATION") == "1" {
		vm := newVM(t)
		wantRoomCovers(t, vm, `URL.canParse("http://\u00e9\u0301.example/")`)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFirstNormalizationCounted$", "-test.count=1")
	cmd.Env = append(os.Environ(), "URL_FIRST_NORMALIZATION=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("in a new process: %v\n%s", err, out)
	}
}

// setGlobal sets the global name of vm to s.
func setGlobal(t testing.TB, vm *quickjs.VM, name, s string) {
	t.Helper()
	g := vm.GlobalObject()
	defer g.Free()
	a, err := vm.NewAtom(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.SetProperty(g, a, s); err != nil {
		t.Fatal(err)
	}
}
