// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// This is where the en-US claims in the package documentation come from. The
// same expressions are evaluated by a full ICU - node's Intl - and by this
// package, and every difference has to fall into one of the known categories
// below. It is the check to run after touching anything in the formatters: the
// spec says what the options mean, but only CLDR knows that a minute beside an
// hour grows a leading zero while a minute alone does not.
//
// It skips when node is missing, so it is a no-op on a builder that has none.

// narrowSpace is U+202F, the narrow no-break space CLDR puts before AM/PM.
const narrowSpace = "\u202f"

// icuDifference is a difference this package makes on purpose, or one that is
// node's rather than ours. Anything not matching one of these fails the test.
var icuDifferences = []struct {
	why   string
	match func(got, want string) bool
}{
	{
		// V8 disagrees with itself here, so one separator has to serve both.
		"the space before AM/PM, which V8 writes as U+202F in formatToParts and as an ordinary space in format",
		func(got, want string) bool {
			return strings.Contains(want, narrowSpace) && strings.ReplaceAll(want, narrowSpace, " ") == got
		},
	},
	{
		// Older CLDR keeps the pre-1995 name as the canonical one.
		"a time zone node's CLDR still canonicalizes to its old name",
		func(got, want string) bool {
			return strings.Contains(want, "Calcutta") && strings.Contains(got, "Kolkata")
		},
	},
	{
		// CLDR's spelled out zone names are not data Go ships.
		"a spelled out time zone name, where this package falls back to the GMT offset",
		func(got, want string) bool {
			return strings.Contains(got, "GMT") && !strings.Contains(want, "GMT") ||
				strings.Contains(got, "Coordinated Universal Time") && !strings.Contains(want, "Coordinated")
		},
	},
	{
		// The formatted output agrees; only the reported cycle differs.
		"hourCycle for hour12:false, which V8 reports as h24 while formatting h23",
		func(got, want string) bool {
			return strings.Contains(want, `"h24"`) && strings.Contains(got, `"h23"`)
		},
	},
}

func TestAgainstICU(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the ICU differential in -short mode")
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("skipping the ICU differential: node not found in $PATH")
	}

	exprs := icuCorpus()
	want, err := icuResults(t, node, exprs)
	if err != nil {
		t.Skip("skipping the ICU differential: ", err)
	}

	vm := newVM(t)
	byCause := map[string]int{}
	for i, expr := range exprs {
		got := icuEval(t, vm, expr)
		if got == want[i] {
			continue
		}

		why := ""
		for _, d := range icuDifferences {
			if d.match(got, want[i]) {
				why = d.why
				break
			}
		}
		byCause[why]++
		if why != "" {
			continue
		}

		if byCause[""] <= 25 {
			t.Errorf("%s\n\tgot  %q\n\tnode %q", expr, got, want[i])
		}
	}
	known := 0
	for why, n := range byCause {
		if why != "" {
			known += n
			t.Logf("%4d known differences: %s", n, why)
		}
	}
	t.Logf("%d of %d expressions match ICU exactly, %d differ for a known reason, %d are unexplained",
		len(exprs)-known-byCause[""], len(exprs), known, byCause[""])
}

// icuEval evaluates expr, returning an error's type rather than failing: the
// two implementations have to agree on what throws, not on the wording.
func icuEval(t *testing.T, vm *quickjs.VM, expr string) string {
	t.Helper()
	v, err := vm.Eval("(() => {try {return String("+expr+")} catch (e) {return 'ERR:' + e.constructor.name}})()", quickjs.EvalGlobal)
	if err != nil {
		return "EVAL FAILED: " + err.Error()
	}

	s, _ := v.(string)
	return s
}

// icuResults evaluates exprs in node, with the same default time zone the VM
// under test uses. A node too old for the options used here reports itself by
// disagreeing with the probe rather than by failing thousands of expressions.
func icuResults(t *testing.T, node string, exprs []string) ([]string, error) {
	t.Helper()
	probe := []string{
		`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(1234)`,
		`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(1.2)`,
		`new Intl.DateTimeFormat('en-US', {timeZone: 'UTC', dateStyle: 'medium'}).format(new Date(0))`,
	}
	want := []string{"1.2K", "2", "Jan 1, 1970"}
	r, err := runNode(t, node, append(probe, exprs...))
	if err != nil {
		return nil, err
	}

	for i, v := range want {
		if r[i] != v {
			return nil, fmt.Errorf("node's Intl answers %q where this harness expects %q, its ICU is not the one this was calibrated against", r[i], v)
		}
	}
	return r[len(probe):], nil
}

func runNode(t *testing.T, node string, exprs []string) ([]string, error) {
	t.Helper()
	b, err := json.Marshal(exprs)
	if err != nil {
		return nil, err
	}

	fn := filepath.Join(t.TempDir(), "corpus.js")
	src := "const exprs = " + string(b) + `;
const out = [];
for (const e of exprs) {
	try {
		out.push(String(eval(e)));
	} catch (err) {
		out.push('ERR:' + err.constructor.name);
	}
}
process.stdout.write(JSON.stringify(out));
`
	if err := os.WriteFile(fn, []byte(src), 0o600); err != nil {
		return nil, err
	}

	cmd := exec.Command(node, fn)
	cmd.Env = append(os.Environ(), "TZ=UTC")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running node: %v", err)
	}

	var r []string
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, err
	}

	return r, nil
}

// icuCorpus returns the expressions to compare: every option this package
// implements, across values and instants chosen to hit the edges - a DST
// switch in both directions, a half hour and a three quarter hour zone, a
// value that rounds up into another magnitude, zero, NaN and both infinities.
func icuCorpus() (r []string) {
	dateOptions := []string{
		`{}`,
		`{dateStyle:'full'}`, `{dateStyle:'long'}`, `{dateStyle:'medium'}`, `{dateStyle:'short'}`,
		`{timeStyle:'full'}`, `{timeStyle:'long'}`, `{timeStyle:'medium'}`, `{timeStyle:'short'}`,
		`{dateStyle:'full',timeStyle:'full'}`, `{dateStyle:'long',timeStyle:'medium'}`,
		`{dateStyle:'medium',timeStyle:'short'}`, `{dateStyle:'short',timeStyle:'short'}`,
		`{year:'numeric'}`, `{year:'2-digit'}`, `{month:'numeric'}`, `{month:'2-digit'}`,
		`{month:'long'}`, `{month:'short'}`, `{month:'narrow'}`, `{day:'numeric'}`,
		`{weekday:'long'}`, `{weekday:'short'}`, `{weekday:'narrow'}`,
		`{year:'numeric',month:'long'}`, `{year:'numeric',month:'numeric'}`,
		`{month:'numeric',day:'numeric'}`, `{month:'long',day:'numeric'}`,
		`{year:'numeric',month:'long',day:'numeric'}`, `{year:'numeric',month:'short',day:'numeric'}`,
		`{year:'numeric',month:'2-digit',day:'2-digit'}`,
		`{weekday:'long',year:'numeric',month:'long',day:'numeric'}`,
		`{weekday:'short',month:'numeric',day:'numeric'}`,
		`{year:'numeric',era:'short'}`, `{year:'numeric',era:'long'}`,
		`{hour:'numeric'}`, `{hour:'2-digit'}`, `{hour:'numeric',minute:'numeric'}`,
		`{hour:'numeric',minute:'2-digit'}`, `{hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{hour:'numeric',minute:'numeric',hour12:false}`, `{hour:'numeric',minute:'numeric',hour12:true}`,
		`{hour:'numeric',minute:'numeric',hourCycle:'h23'}`, `{hour:'numeric',minute:'numeric',hourCycle:'h11'}`,
		`{hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}`,
		`{hour:'numeric',minute:'numeric',second:'numeric',fractionalSecondDigits:3}`,
		`{hour:'numeric',minute:'numeric',second:'numeric',fractionalSecondDigits:1}`,
		`{minute:'numeric',second:'numeric'}`,
		`{timeZoneName:'short'}`, `{timeZoneName:'shortOffset'}`, `{timeZoneName:'longOffset'}`,
		`{hour:'numeric',timeZoneName:'short'}`,
		`{year:'numeric',month:'numeric',day:'numeric',hour:'numeric',minute:'numeric',second:'numeric'}`,
		`{month:'long',day:'numeric',hour:'numeric'}`, `{month:'short',day:'numeric',hour:'numeric'}`,
		`{dayPeriod:'short'}`, `{hour:'numeric',dayPeriod:'short'}`,
	}
	instants := []string{
		"0",
		"1000000000000",
		"1755300000000",
		"1741503600000", // 2025-03-09T07:00:00Z, a US DST switch.
		"1762063200000", // 2025-11-02T06:00:00Z, back again.
		"1500",
		"-1000000000000",
	}
	zones := []string{
		"'UTC'", "'America/New_York'", "'Europe/Prague'",
		"'Asia/Kolkata'",     // A half hour offset.
		"'Australia/Sydney'", // DST the other way around.
		"'Pacific/Chatham'",  // Three quarters of an hour.
	}
	for i, opts := range dateOptions {
		for j, at := range instants {
			o := strings.TrimSuffix(opts, "}")
			if o != "{" {
				o += ","
			}
			o += "timeZone:" + zones[(i+j)%len(zones)] + "}"
			r = append(r,
				fmt.Sprintf(`new Intl.DateTimeFormat('en-US',%s).format(new Date(%s))`, o, at),
				fmt.Sprintf(`JSON.stringify(new Intl.DateTimeFormat('en-US',%s).formatToParts(new Date(%s)))`, o, at),
				fmt.Sprintf(`JSON.stringify(new Intl.DateTimeFormat('en-US',%s).resolvedOptions())`, o),
			)
		}
	}
	for _, opts := range dateOptions {
		for _, m := range []string{"toLocaleString", "toLocaleDateString", "toLocaleTimeString"} {
			r = append(r, fmt.Sprintf(`new Date(1755300000000).%s('en-US',%s)`, m, opts))
		}
	}

	numberOptions := []string{
		`{}`, `{useGrouping:false}`, `{useGrouping:'min2'}`, `{useGrouping:'always'}`,
		`{minimumFractionDigits:2}`, `{maximumFractionDigits:0}`, `{maximumFractionDigits:1}`,
		`{maximumFractionDigits:5}`, `{minimumFractionDigits:2,maximumFractionDigits:4}`,
		`{minimumIntegerDigits:3}`, `{minimumSignificantDigits:3}`, `{maximumSignificantDigits:2}`,
		`{maximumSignificantDigits:4}`, `{minimumSignificantDigits:2,maximumSignificantDigits:5}`,
		`{style:'percent'}`, `{style:'percent',maximumFractionDigits:2}`, `{style:'percent',minimumFractionDigits:1}`,
		`{style:'currency',currency:'USD'}`, `{style:'currency',currency:'EUR'}`,
		`{style:'currency',currency:'JPY'}`, `{style:'currency',currency:'GBP'}`,
		`{style:'currency',currency:'CAD'}`, `{style:'currency',currency:'CAD',currencyDisplay:'narrowSymbol'}`,
		`{style:'currency',currency:'USD',currencyDisplay:'code'}`,
		`{style:'currency',currency:'USD',currencySign:'accounting'}`, `{style:'currency',currency:'BHD'}`,
		`{style:'unit',unit:'liter'}`, `{style:'unit',unit:'kilometer'}`,
		`{style:'unit',unit:'kilometer-per-hour'}`, `{style:'unit',unit:'byte'}`,
		`{style:'unit',unit:'megabyte',unitDisplay:'long'}`, `{style:'unit',unit:'day',unitDisplay:'long'}`,
		`{style:'unit',unit:'celsius',unitDisplay:'narrow'}`,
		`{notation:'compact'}`, `{notation:'compact',compactDisplay:'long'}`,
		`{notation:'scientific'}`, `{notation:'engineering'}`,
		`{signDisplay:'always'}`, `{signDisplay:'never'}`, `{signDisplay:'exceptZero'}`, `{signDisplay:'negative'}`,
		`{maximumFractionDigits:0,roundingMode:'ceil'}`, `{maximumFractionDigits:0,roundingMode:'floor'}`,
		`{maximumFractionDigits:0,roundingMode:'expand'}`, `{maximumFractionDigits:0,roundingMode:'trunc'}`,
		`{maximumFractionDigits:0,roundingMode:'halfCeil'}`, `{maximumFractionDigits:0,roundingMode:'halfFloor'}`,
		`{maximumFractionDigits:0,roundingMode:'halfTrunc'}`, `{maximumFractionDigits:0,roundingMode:'halfEven'}`,
		`{maximumFractionDigits:2,roundingMode:'halfEven'}`, `{maximumFractionDigits:2,roundingMode:'expand'}`,
		`{minimumFractionDigits:2,trailingZeroDisplay:'stripIfInteger'}`,
		`{maximumSignificantDigits:3,roundingMode:'trunc'}`,
	}
	values := []string{
		"0", "-0", "1", "-1", "0.5", "1.005", "2.5", "3.5", "1234.5678", "1234567.891",
		"999999", "1000", "1e21", "1e-7", "123456789", "-1234.5", "NaN", "Infinity",
		"-Infinity", "0.0001", "12345", "99.995", "1e6", "1e9",
	}
	for _, opts := range numberOptions {
		for _, v := range values {
			r = append(r,
				fmt.Sprintf(`new Intl.NumberFormat('en-US',%s).format(%s)`, opts, v),
				fmt.Sprintf(`(%s).toLocaleString('en-US',%s)`, v, opts),
			)
		}
		r = append(r,
			fmt.Sprintf(`JSON.stringify(new Intl.NumberFormat('en-US',%s).resolvedOptions())`, opts),
			fmt.Sprintf(`JSON.stringify(new Intl.NumberFormat('en-US',%s).formatToParts(-1234.5678))`, opts),
		)
	}
	return r
}
