// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The engine is generated with a profile of tifa's pages (engine/pgo, see
// docs/changes/generator.md, "A profile of tifa's pages"). The profile tells
// llc where to place blocks and spills, which is the whole gain. What is
// inlined stays what the build without the profile inlines: make gen compiles
// twice, first without the profile to write the inlining decisions down
// (InlineRemarks), then with the profile, replaying them (ReplayClangArgs,
// ReplayOptArgs). Left to itself the profile inlines other functions into the
// register loop, which grows 3.4 times and takes a Go frame past the rule.

// PGOStaleMax is how many functions of the engine may have a profile that no
// longer fits their C before make gen warns that the profile is old: a few are
// the normal state between two trainings.
const PGOStaleMax = 30

// PGORequired are the functions make gen names when their profile no longer
// fits their C: the register loop is most of the gain, and a change of its C
// drops its whole profile.
var PGORequired = []string{"JS_RegvmRun"}

// RemarksFlags are the flags of the first compile: the inlining decisions as
// remarks, with the line and column of each call site, which the replay names.
var RemarksFlags = []string{"-gline-tables-only", "-Rpass=inline"}

// PGOPinned are the options of LLVM that keep a decision from the profile
// whose cost depends on the data (docs/changes/generator.md, what the profile
// decides).
var PGOPinned = []string{
	// No size optimization of code the training ran little or never: code
	// cold in tifa's pages but hot elsewhere (loading a bundle: +6% to +8%
	// instructions, test262: +2.7%) would get slower.
	"-pgso=false",
}

// ProfileFlags are the flags of the second compile besides the replay. The
// line tables keep the call sites of the replay matching those of the remarks
// and are stripped after the post-link optimizer. -Wmisexpect names each
// likely() or unlikely() the training ran the other way more often than not
// (the tolerance of 50 percent), see MisexpectLines.
func ProfileFlags(indexed string) []string {
	f := []string{"-gline-tables-only", "-fprofile-instr-use=" + indexed, "-Wno-profile-instr-unprofiled",
		"-Wmisexpect", "-fdiagnostics-misexpect-tolerance=50"}
	for _, o := range PGOPinned {
		f = append(f, "-mllvm", o)
	}
	return f
}

// PGOOptFlags pin the post-link optimizer as ProfileFlags pin clang, and
// PGOLLCFlags turn size optimization of cold code off in llc.
var (
	PGOOptFlags = PGOPinned
	PGOLLCFlags = []string{"-pgso=false"}
)

// MisexpectLines keeps the warnings of -Wmisexpect from the output of clang:
// a likely() or unlikely() of the C that the training ran the other way.
func MisexpectLines(stderr string) []string {
	var out []string
	for _, l := range strings.Split(stderr, "\n") {
		if strings.HasSuffix(l, "[-Wmisexpect]") {
			out = append(out, l)
		}
	}
	return out
}

// InlineRemarks keeps the lines of remarks that say a call was inlined: the
// input of -cgscc-inline-replay. The rest (the remarks of calls not inlined,
// the source lines clang prints under each remark, warnings) goes.
func InlineRemarks(stderr string) string {
	var b strings.Builder
	for _, l := range strings.Split(stderr, "\n") {
		if strings.Contains(l, "' inlined into '") {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ReplayClangArgs makes clang inline exactly the calls of remarks and no other.
func ReplayClangArgs(remarks string) []string {
	return []string{"-mllvm", "-cgscc-inline-replay=" + remarks,
		"-mllvm", "-cgscc-inline-replay-scope=Module", "-mllvm", "-cgscc-inline-replay-fallback=NeverInline"}
}

// ReplayOptArgs is ReplayClangArgs for opt.
func ReplayOptArgs(remarks string) []string {
	return []string{"-cgscc-inline-replay=" + remarks,
		"-cgscc-inline-replay-scope=Module", "-cgscc-inline-replay-fallback=NeverInline"}
}

var reCGProfile = regexp.MustCompile(`(?m)^!(\d+) = !\{i32 \d+, !"CG Profile", .*\n`)

// StripCGProfile takes the "CG Profile" module flag out of a module. clang
// writes one into each file it compiles with a profile and llvm-link merges
// them, then the post-link optimizer adds a second, which the verifier refuses
// (two flags of one name). Left in after the optimizer, it makes llc write
// .cg_profile directives, which the lowering does not take. LLVM 22 has no
// switch for it. The flag only orders functions for a linker, which the Go
// linker is not.
func StripCGProfile(ll string) string {
	m := reCGProfile.FindStringSubmatch(ll)
	if m == nil {
		return ll
	}
	n := "!" + m[1]
	ll = strings.Replace(ll, m[0], "", 1)
	re := regexp.MustCompile(`(?m)^(!llvm\.module\.flags = !\{)(.*)\}$`)
	return re.ReplaceAllStringFunc(ll, func(line string) string {
		sm := re.FindStringSubmatch(line)
		var keep []string
		for _, f := range strings.Split(sm[2], ", ") {
			if f != n {
				keep = append(keep, f)
			}
		}
		return sm[1] + strings.Join(keep, ", ") + "}"
	})
}

// ProfileNames lists the functions of a text profile (llvm-profdata merge
// --text): the line before each "# Func Hash:". A static function is named
// file:name, as !PGOFuncName names it.
func ProfileNames(text string) map[string]bool {
	names := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	prev := ""
	for sc.Scan() {
		l := sc.Text()
		if l == "# Func Hash:" && prev != "" {
			names[prev] = true
		}
		prev = l
	}
	return names
}

var (
	reDefinePGO  = regexp.MustCompile(`(?m)^define [^@]*@("[^"]+"|[\w.$]+)\(.*\{$`)
	rePGOName    = regexp.MustCompile(`!PGOFuncName !(\d+)`)
	reMDString   = regexp.MustCompile(`(?m)^!(\d+) = !\{!"([^"]*)"\}$`)
	reHasProfile = regexp.MustCompile(`\s!prof !\d+`)
)

// UnprofiledFunctions reads the IR clang writes with a profile before any
// optimization (-Xclang -disable-llvm-passes) and gives the functions that
// have no profile: those the profile names have one that no longer fits their
// C (stale), the others are new since the training. Names are the profile's:
// file:name for a static function.
func UnprofiledFunctions(feIR string, profile map[string]bool) (stale, fresh []string) {
	md := map[string]string{}
	for _, m := range reMDString.FindAllStringSubmatch(feIR, -1) {
		md[m[1]] = m[2]
	}
	for _, m := range reDefinePGO.FindAllStringSubmatch(feIR, -1) {
		line := m[0]
		if reHasProfile.MatchString(line) {
			continue
		}
		name := strings.Trim(m[1], `"`)
		if p := rePGOName.FindStringSubmatch(line); p != nil {
			name = md[p[1]]
		}
		if profile[name] {
			stale = append(stale, name)
		} else {
			fresh = append(fresh, name)
		}
	}
	sort.Strings(stale)
	sort.Strings(fresh)
	return stale, fresh
}

// BareName is a profile name without the file of a static function.
func BareName(name string) string {
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// StaleWarnings are the lines make gen prints when the profile is behind the C:
// a function of required that has no profile (its C changed since the training,
// or it is new), and more than max functions whose C changed. make gen goes on:
// a function without a profile compiles as the build without one does and only
// loses the gain. Before a release a stale JS_RegvmRun is trained again
// (docs/releasing.md).
func StaleWarnings(stale, fresh, required []string, max int) []string {
	var w []string
	for _, r := range required {
		for _, s := range append(append([]string{}, stale...), fresh...) {
			if BareName(s) == r {
				w = append(w, fmt.Sprintf("warning: profile: %s has no profile, its C changed since the training or it is new: it loses the gain until engine/pgo/train.sh runs again (docs/upgrading.md)", r))
				break
			}
		}
	}
	if len(stale) > max {
		w = append(w, fmt.Sprintf("warning: profile: the C of %d functions changed since the training (more than %d): the profile is old, engine/pgo/train.sh makes a new one (docs/upgrading.md)", len(stale), max))
	}
	return w
}
