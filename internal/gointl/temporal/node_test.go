//go:build gointl_all

package temporal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestTemporalMatchesNode replays testdata/temporal_node.js: Temporal's
// operations as JavaScript calls them, through a replay of what V8 does
// with JavaScript's values before it hands them to temporal_rs.
func TestTemporalMatchesNode(t *testing.T) {
	f, err := os.Open("../testdata/temporal_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	h, err := newHarness()
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(nil, 1<<20)
	ran, bad := map[string]int{}, map[string]int{}
	for s.Scan() {
		line := s.Bytes()
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal(line, &c); err != nil || len(c) != 4 {
			t.Fatalf("%s: %v", line, err)
		}
		var op string
		json.Unmarshal(c[0], &op)
		ran[op]++
		recv, err := decodeValue(c[1])
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var rawArgs []json.RawMessage
		json.Unmarshal(c[2], &rawArgs)
		var args []any
		for _, a := range rawArgs {
			v, err := decodeValue(a)
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			args = append(args, v)
		}
		got := h.run(op, recv, args)
		var want any
		if err := json.Unmarshal(c[3], &want); err != nil {
			t.Fatal(err)
		}
		gotJSON, _ := json.Marshal(got)
		var gotNorm any
		json.Unmarshal(gotJSON, &gotNorm)
		if !jsonEqual(gotNorm, want) {
			bad[op]++
			if bad[op] <= 8 {
				t.Errorf("%s %s %s: got %s, want %s", op, c[1], c[2], gotJSON, c[3])
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	var ops []string
	total := 0
	for op, n := range ran {
		total += n
		if bad[op] > 0 {
			ops = append(ops, op)
		}
	}
	sort.Strings(ops)
	for _, op := range ops {
		t.Logf("%s: %d of %d differ", op, bad[op], ran[op])
	}
	t.Logf("%d cases", total)
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// ==== JavaScript values ====

type jsUndefined struct{}
type jsNull struct{}

// jsObject is a plain object.
type jsObject struct {
	keys []string
	vals map[string]any
}

func (o *jsObject) get(k string) any {
	if o == nil {
		return jsUndefined{}
	}
	if v, ok := o.vals[k]; ok {
		return v
	}
	return jsUndefined{}
}

func newObject(kv ...any) *jsObject {
	o := &jsObject{vals: map[string]any{}}
	for i := 0; i < len(kv); i += 2 {
		k := kv[i].(string)
		o.keys = append(o.keys, k)
		o.vals[k] = kv[i+1]
	}
	return o
}

// jsError is what was thrown.
type jsError struct{ name, msg string }

func (e *jsError) Error() string { return e.name + ": " + e.msg }

func typeErr(msg string) error  { return &jsError{"TypeError", "Temporal error: " + msg} }
func rangeErr(msg string) error { return &jsError{"RangeError", "Temporal error: " + msg} }

// rustErr is the error V8 throws for a temporal_rs error.
func rustErr(err error) error {
	if err == nil {
		return nil
	}
	var je *jsError
	if errors.As(err, &je) {
		return err
	}
	msg := err.Error()
	name := "Error"
	switch {
	case errors.Is(err, ErrRange):
		name, msg = "RangeError", strings.TrimPrefix(msg, "RangeError: ")
	case errors.Is(err, ErrType):
		name, msg = "TypeError", strings.TrimPrefix(msg, "TypeError: ")
	default:
		return &jsError{"Error", "Temporal error: Internal error: " + strings.TrimPrefix(msg, "Error: ") + "."}
	}
	return &jsError{name, "Temporal error: " + msg}
}

// decodeValue reads the recording's notation for a value.
func decodeValue(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	return decodeNext(d)
}

func decodeNext(d *json.Decoder) (any, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch v := tok.(type) {
	case json.Number:
		return strconv.ParseFloat(string(v), 64)
	case string:
		return v, nil
	case bool:
		return v, nil
	case nil:
		return jsNull{}, nil
	case json.Delim:
		if v == '[' {
			var out []any
			for d.More() {
				e, err := decodeNext(d)
				if err != nil {
					return nil, err
				}
				out = append(out, e)
			}
			d.Token()
			return out, nil
		}
		o := &jsObject{vals: map[string]any{}}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return nil, err
			}
			e, err := decodeNext(d)
			if err != nil {
				return nil, err
			}
			o.keys = append(o.keys, k.(string))
			o.vals[k.(string)] = e
		}
		d.Token()
		if len(o.keys) == 1 && strings.HasPrefix(o.keys[0], "$") {
			return specialValue{o.keys[0][1:], o.vals[o.keys[0]]}, nil
		}
		return o, nil
	}
	return nil, fmt.Errorf("unexpected %v", tok)
}

// specialValue is one of the notation's $ objects, resolved by the harness.
type specialValue struct {
	kind string
	val  any
}

// ==== The harness ====

type harness struct {
	data  *Data
	zones *Zones
	opm   map[string]opFunc
}

func newHarness() (*harness, error) {
	d, err := LoadData(intl.Embedded)
	if err != nil {
		return nil, err
	}
	h := &harness{data: d, zones: d.Zones}
	h.opm = map[string]opFunc{}
	h.durationOps(h.opm)
	h.fromOps(h.opm)
	h.plainDateOps(h.opm)
	h.plainTimeOps(h.opm)
	h.plainDateTimeOps(h.opm)
	h.yearMonthOps(h.opm)
	h.instantOps(h.opm)
	h.zonedOps(h.opm)
	h.jsonOps(h.opm)
	return h, nil
}

func (h *harness) cal(id string) *Calendar {
	c, _ := h.data.Calendar(id)
	return c
}

// resolve turns the notation's $ objects into values.
func (h *harness) resolve(v any) (any, error) {
	switch x := v.(type) {
	case specialValue:
		switch x.kind {
		case "u":
			return jsUndefined{}, nil
		case "n":
			f, _ := strconv.ParseFloat(strings.Replace(x.val.(string), "Infinity", "Inf", 1), 64)
			if x.val.(string) == "-0" {
				f = math.Copysign(0, -1)
			}
			return f, nil
		case "big":
			b, _ := new(big.Int).SetString(x.val.(string), 10)
			return b, nil
		}
		return h.call("static:"+x.kind+".from", jsUndefined{}, []any{x.val})
	case *jsObject:
		o := &jsObject{keys: x.keys, vals: map[string]any{}}
		for k, e := range x.vals {
			r, err := h.resolve(e)
			if err != nil {
				return nil, err
			}
			o.vals[k] = r
		}
		return o, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			r, err := h.resolve(e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

func (h *harness) run(op string, recv any, args []any) any {
	r, err := h.resolve(recv)
	if err != nil {
		return encodeError(err)
	}
	var a []any
	for _, x := range args {
		v, err := h.resolve(x)
		if err != nil {
			return encodeError(err)
		}
		a = append(a, v)
	}
	out, err := h.call(op, r, a)
	if err != nil {
		return encodeError(err)
	}
	return encodeResult(out)
}

func encodeError(err error) any {
	var je *jsError
	if !errors.As(err, &je) {
		je = rustErr(err).(*jsError)
	}
	return map[string]any{"$err": je.name, "msg": je.msg}
}

func encodeResult(v any) any {
	switch x := v.(type) {
	case jsUndefined:
		return map[string]any{"$u": 0}
	case jsNull:
		return nil
	case float64:
		switch {
		case math.IsNaN(x):
			return map[string]any{"$n": "NaN"}
		case math.IsInf(x, 1):
			return map[string]any{"$n": "Infinity"}
		case math.IsInf(x, -1):
			return map[string]any{"$n": "-Infinity"}
		case x == 0 && math.Signbit(x):
			return map[string]any{"$n": "-0"}
		}
		return x
	case int:
		return float64(x)
	case *big.Int:
		return map[string]any{"$big": x.String()}
	case Duration:
		s, _ := x.String(DefaultToStringOptions())
		return map[string]any{"$Duration": s}
	case Instant:
		return map[string]any{"$Instant": x.stringUTC()}
	case PlainTime:
		s, _ := x.String(DefaultToStringOptions())
		return map[string]any{"$PlainTime": s}
	case PlainDate:
		return map[string]any{"$PlainDate": x.String(CalendarAlways)}
	case PlainDateTime:
		s, _ := x.String(DefaultToStringOptions(), CalendarAlways)
		return map[string]any{"$PlainDateTime": s}
	case PlainYearMonth:
		return map[string]any{"$PlainYearMonth": x.String(CalendarAlways)}
	case PlainMonthDay:
		return map[string]any{"$PlainMonthDay": x.String(CalendarAlways)}
	case ZonedDateTime:
		s, _ := x.String(OffsetAuto, TimeZoneAuto, CalendarAlways, DefaultToStringOptions())
		return map[string]any{"$ZonedDateTime": s + " " + x.Offset()}
	}
	return v
}

func (i Instant) stringUTC() string {
	var b ixdtfBuilder
	dt := isoDateTimeFromEpochNanoseconds(i.ns, 0)
	b.date(dt.Date)
	b.time(dt.Time, PrecisionAuto)
	b.z(OffsetAuto)
	return b.String()
}

// ==== ECMAScript's conversions ====

var numberLiteral = regexp.MustCompile(`^[+-]?(Infinity|(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?)$`)

// jsTrim is StringToNumber's trimming of white space and line terminators.
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
			return true
		}
		return r >= 0x2000 && r <= 0x200A
	})
}

func stringToNumber(s string) float64 {
	s = jsTrim(s)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			b, ok := new(big.Int).SetString(s[2:], base)
			if !ok {
				return math.NaN()
			}
			f, _ := new(big.Float).SetInt(b).Float64()
			return f
		}
	}
	if !numberLiteral.MatchString(s) {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(strings.Replace(s, "Infinity", "Inf", 1), 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return math.NaN()
	}
	return f
}

func toNumber(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		return stringToNumber(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	case jsNull:
		return 0, nil
	case jsUndefined:
		return math.NaN(), nil
	case *big.Int:
		return 0, &jsError{"TypeError", "Cannot convert a BigInt value to a number"}
	}
	return math.NaN(), nil
}

// numberToString is Number::toString for the values the recording has.
func numberToString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	if math.Abs(f) < 1e21 && math.Abs(f) >= 1e-6 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	s := strconv.FormatFloat(f, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(exp)
	if e > 0 {
		return mant + "e+" + strconv.Itoa(e)
	}
	return mant + "e" + strconv.Itoa(e)
}

func toString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return numberToString(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case jsNull:
		return "null", nil
	case jsUndefined:
		return "undefined", nil
	case *big.Int:
		return x.String(), nil
	case *jsObject:
		return "[object Object]", nil
	}
	return "", fmt.Errorf("toString of %T", v)
}

func isObject(v any) bool {
	switch v.(type) {
	case string, float64, bool, jsNull, jsUndefined, *big.Int:
		return false
	}
	return true
}

func isUndefined(v any) bool { _, ok := v.(jsUndefined); return ok }

func toIntegerWithTruncation(v any) (float64, error) {
	n, err := toNumber(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, rangeErr("Expected finite integer.")
	}
	return math.Trunc(n) + 0, nil
}

func toIntegerIfIntegral(v any) (float64, error) {
	n, err := toNumber(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n {
		return 0, rangeErr("Expected finite integer.")
	}
	return n, nil
}

func toPositiveIntegerWithTruncation(v any) (float64, error) {
	n, err := toIntegerWithTruncation(v)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, rangeErr("Expected positive integer.")
	}
	return n, nil
}

// inRange is base::IsValueInRangeForNumericType for an integral double.
func inRange(f, lo, hi float64) bool { return f >= lo && f <= hi }

// jsBytes is the bytes V8 hands temporal_rs for a string (HandleStringEncodings).
func jsBytes(s string) ([]byte, error) {
	b, err := JSStringBytes(utf16.Encode([]rune(s)))
	if err != nil {
		return nil, rustErr(err)
	}
	return b, nil
}

// ==== Options ====

func getOptionsObject(v any) (*jsObject, error) {
	switch x := v.(type) {
	case jsUndefined:
		return &jsObject{vals: map[string]any{}}, nil
	case *jsObject:
		return x, nil
	}
	if isObject(v) {
		return &jsObject{vals: map[string]any{}}, nil
	}
	return nil, &jsError{"TypeError", "invalid_argument"}
}

func get(o any, key string) any {
	if obj, ok := o.(*jsObject); ok {
		return obj.get(key)
	}
	return jsUndefined{}
}

// getStringOption is V8's GetStringOption over a list of values: the
// index of the value, def where the option is absent, and -1 for none
// (required).
func getStringOption(o *jsObject, key, method string, values []string, def int) (int, error) {
	v := o.get(key)
	if isUndefined(v) {
		if def >= 0 {
			return def, nil
		}
		return 0, &jsError{"RangeError", fmt.Sprintf("Value undefined out of range for %s options property %s", method, key)}
	}
	s, err := toString(v)
	if err != nil {
		return 0, err
	}
	for i, x := range values {
		if s == x {
			return i, nil
		}
	}
	return 0, &jsError{"RangeError", fmt.Sprintf("Value %s out of range for %s options property %s", s, method, key)}
}

var unitStrings = []string{"year", "month", "week", "day", "hour", "minute", "second", "millisecond",
	"microsecond", "nanosecond", "auto", "years", "months", "weeks", "days", "hours", "minutes", "seconds",
	"milliseconds", "microseconds", "nanoseconds"}

// getUnitOption is GetTemporalUnitValuedOption: NoUnit where unset.
func getUnitOption(o *jsObject, key, method string, required bool) (Unit, error) {
	def := len(unitStrings)
	if required {
		def = -1
	}
	i, err := getStringOption(o, key, method, unitStrings, def)
	if err != nil {
		return 0, err
	}
	if i == len(unitStrings) {
		return NoUnit, nil
	}
	u, _ := ParseUnit(unitStrings[i])
	return u, nil
}

// validateUnit is ValidateTemporalUnitValue.
func validateUnit(u Unit, group unitGroup, extra Unit) error {
	if u == NoUnit || u == extra && extra != NoUnit {
		return nil
	}
	if u == UnitAuto {
		return rangeErr("Auto unit not allowed here")
	}
	if u.isDateUnit() {
		if group == groupDate || group == groupDateTime {
			return nil
		}
		return rangeErr("Found date unit, expect time unit")
	}
	if group == groupTime || group == groupDateTime {
		return nil
	}
	return rangeErr("Found date unit, expect time unit")
}

func getRoundingIncrement(o *jsObject) (RoundingIncrement, error) {
	v := o.get("roundingIncrement")
	if isUndefined(v) {
		return 1, nil
	}
	n, err := toIntegerWithTruncation(v)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > 1e9 {
		return 0, rangeErr("Integer out of range.")
	}
	return RoundingIncrement(n), nil
}

var modeStrings = []string{"ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand",
	"halfTrunc", "halfEven"}

func getRoundingMode(o *jsObject, method string, def RoundingMode) (RoundingMode, error) {
	i, err := getStringOption(o, "roundingMode", method, modeStrings, int(def-Ceil))
	if err != nil {
		return 0, err
	}
	return RoundingMode(i) + Ceil, nil
}

func getFractionalSecondDigits(o *jsObject) (Precision, error) {
	v := o.get("fractionalSecondDigits")
	if isUndefined(v) {
		return PrecisionAuto, nil
	}
	n, ok := v.(float64)
	if !ok {
		s, err := toString(v)
		if err != nil {
			return 0, err
		}
		if s != "auto" {
			return 0, &jsError{"RangeError", "fractionalSecondDigits value is out of range."}
		}
		return PrecisionAuto, nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, &jsError{"RangeError", "fractionalSecondDigits value is out of range."}
	}
	d := math.Floor(n)
	if d < 0 || d > 9 {
		return 0, &jsError{"RangeError", "fractionalSecondDigits value is out of range."}
	}
	return Precision(d), nil
}

func getOverflow(opts any, method string) (Overflow, error) {
	if isUndefined(opts) {
		return Constrain, nil
	}
	o, ok := opts.(*jsObject)
	if !ok {
		if isObject(opts) {
			return Constrain, nil
		}
		return 0, typeErrWithArg("Option must be object:", "overflow")
	}
	i, err := getStringOption(o, "overflow", method, []string{"constrain", "reject"}, 0)
	return Overflow(i), err
}

func typeErrWithArg(msg, arg string) error {
	return &jsError{"TypeError", "Temporal error: " + msg + " " + arg + "."}
}

// ==== Durations ====

func (h *harness) toDuration(v any) (Duration, error) {
	switch x := v.(type) {
	case Duration:
		return x, nil
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return Duration{}, err
		}
		d, err := ParseDuration(b)
		return d, rustErr(err)
	}
	if !isObject(v) {
		return Duration{}, typeErr("Duration argument must be Duration or string.")
	}
	p, err := h.partialDuration(v)
	if err != nil {
		return Duration{}, err
	}
	d, err := DurationFromPartial(p)
	return d, rustErr(err)
}

func (h *harness) partialDuration(v any) (PartialDuration, error) {
	var p PartialDuration
	if !isObject(v) {
		return p, typeErr("Must provide a duration.")
	}
	intField := func(key string, out **int64) error {
		x := get(v, key)
		if isUndefined(x) {
			return nil
		}
		f, err := toIntegerIfIntegral(x)
		if err != nil {
			return err
		}
		if !(f >= -0x1p63 && f < 0x1p63) {
			return rangeErr("Duration field out of range.")
		}
		i := int64(f)
		*out = &i
		return nil
	}
	floatField := func(key string, out **float64) error {
		x := get(v, key)
		if isUndefined(x) {
			return nil
		}
		f, err := toIntegerIfIntegral(x)
		if err != nil {
			return err
		}
		*out = &f
		return nil
	}
	for _, e := range []error{intField("days", &p.Days), intField("hours", &p.Hours),
		floatField("microseconds", &p.Microseconds), intField("milliseconds", &p.Milliseconds),
		intField("minutes", &p.Minutes), intField("months", &p.Months), floatField("nanoseconds", &p.Nanoseconds),
		intField("seconds", &p.Seconds), intField("weeks", &p.Weeks), intField("years", &p.Years)} {
		if e != nil {
			return p, e
		}
	}
	if p == (PartialDuration{}) {
		return p, typeErr("Did not provide any valid Duration fields.")
	}
	return p, nil
}

func (h *harness) call(op string, this any, args []any) (any, error) {
	arg := func(i int) any {
		if i < len(args) {
			return args[i]
		}
		return jsUndefined{}
	}
	if fn, ok := h.opm[op]; ok {
		return fn(this, arg)
	}
	return nil, fmt.Errorf("the harness has no %s", op)
}

type opFunc func(this any, arg func(int) any) (any, error)

func (h *harness) durationOps(m map[string]opFunc) {
	m["new:Duration"] = func(_ any, arg func(int) any) (any, error) {
		var f [10]float64
		for i := 0; i < 10; i++ {
			if isUndefined(arg(i)) {
				continue
			}
			n, err := toIntegerIfIntegral(arg(i))
			if err != nil {
				return nil, err
			}
			if i < 8 && !(n >= -0x1p63 && n < 0x1p63) {
				return nil, rangeErr("Integer out of range.")
			}
			f[i] = n
		}
		d, err := DurationFromNumbers(f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9])
		return d, rustErr(err)
	}
	m["static:Duration.from"] = func(_ any, arg func(int) any) (any, error) { return h.toDuration(arg(0)) }
	getters := map[string]func(Duration) any{
		"years": func(d Duration) any { return float64(d.Years()) }, "months": func(d Duration) any { return float64(d.Months()) },
		"weeks": func(d Duration) any { return float64(d.Weeks()) }, "days": func(d Duration) any { return float64(d.Days()) },
		"hours": func(d Duration) any { return float64(d.Hours()) }, "minutes": func(d Duration) any { return float64(d.Minutes()) },
		"seconds":      func(d Duration) any { return float64(d.Seconds()) },
		"milliseconds": func(d Duration) any { return float64(d.Milliseconds()) },
		"microseconds": func(d Duration) any { return d.Microseconds() }, "nanoseconds": func(d Duration) any { return d.Nanoseconds() },
		"sign": func(d Duration) any { return float64(d.Sign()) }, "blank": func(d Duration) any { return d.IsZero() },
	}
	for name, g := range getters {
		g := g
		m["get:Duration."+name] = func(this any, _ func(int) any) (any, error) { return g(this.(Duration)), nil }
	}
	m["method:Duration.negated"] = func(this any, _ func(int) any) (any, error) { return this.(Duration).Negated(), nil }
	m["method:Duration.abs"] = func(this any, _ func(int) any) (any, error) { return this.(Duration).Abs(), nil }
	m["method:Duration.toJSON"] = func(this any, _ func(int) any) (any, error) {
		s, err := this.(Duration).String(DefaultToStringOptions())
		return s, rustErr(err)
	}
	m["method:Duration.add"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toDuration(arg(0))
		if err != nil {
			return nil, err
		}
		d, err := this.(Duration).Add(o)
		return d, rustErr(err)
	}
	m["method:Duration.subtract"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toDuration(arg(0))
		if err != nil {
			return nil, err
		}
		d, err := this.(Duration).Subtract(o)
		return d, rustErr(err)
	}
	m["method:Duration.with"] = func(this any, arg func(int) any) (any, error) {
		d := this.(Duration)
		p, err := h.partialDuration(arg(0))
		if err != nil {
			return nil, err
		}
		set := func(p **int64, v int64) {
			if *p == nil {
				*p = &v
			}
		}
		setF := func(p **float64, v float64) {
			if *p == nil {
				*p = &v
			}
		}
		set(&p.Years, d.Years())
		set(&p.Months, d.Months())
		set(&p.Weeks, d.Weeks())
		set(&p.Days, d.Days())
		set(&p.Hours, d.Hours())
		set(&p.Minutes, d.Minutes())
		set(&p.Seconds, d.Seconds())
		set(&p.Milliseconds, d.Milliseconds())
		setF(&p.Microseconds, d.Microseconds())
		setF(&p.Nanoseconds, d.Nanoseconds())
		out, err := DurationFromPartial(p)
		return out, rustErr(err)
	}
	m["method:Duration.round"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.Duration.prototype.round"
		v := arg(0)
		if isUndefined(v) {
			return nil, typeErr("Must specify a roundTo parameter.")
		}
		var o *jsObject
		switch x := v.(type) {
		case string:
			o = newObject("smallestUnit", x)
		case *jsObject:
			o = x
		default:
			if !isObject(v) {
				return nil, typeErr("roundTo must be an object.")
			}
			o = newObject()
		}
		largest, err := getUnitOption(o, "largestUnit", method, false)
		if err != nil {
			return nil, err
		}
		rel, err := h.relativeTo(o)
		if err != nil {
			return nil, err
		}
		inc, err := getRoundingIncrement(o)
		if err != nil {
			return nil, err
		}
		mode, err := getRoundingMode(o, method, HalfExpand)
		if err != nil {
			return nil, err
		}
		smallest, err := getUnitOption(o, "smallestUnit", method, false)
		if err != nil {
			return nil, err
		}
		if err := validateUnit(smallest, groupDateTime, NoUnit); err != nil {
			return nil, err
		}
		d, err := this.(Duration).Round(RoundingOptions{LargestUnit: largest, SmallestUnit: smallest, RoundingMode: mode, Increment: inc, Compat: intl.NodeICU}, rel)
		return d, rustErr(err)
	}
	m["method:Duration.total"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.Duration.prototype.total"
		v := arg(0)
		if isUndefined(v) {
			return nil, typeErr("Must specify a totalOf parameter")
		}
		var o *jsObject
		switch x := v.(type) {
		case string:
			o = newObject("unit", x)
		case *jsObject:
			o = x
		default:
			if !isObject(v) {
				return nil, typeErr("totalOf must be an object.")
			}
			o = newObject()
		}
		rel, err := h.relativeTo(o)
		if err != nil {
			return nil, err
		}
		u, err := getUnitOption(o, "unit", method, true)
		if err != nil {
			return nil, err
		}
		if err := validateUnit(u, groupDateTime, NoUnit); err != nil {
			return nil, err
		}
		f, err := this.(Duration).Total(u, rel)
		return f, rustErr(err)
	}
	m["static:Duration.compare"] = func(_ any, arg func(int) any) (any, error) {
		one, err := h.toDuration(arg(0))
		if err != nil {
			return nil, err
		}
		two, err := h.toDuration(arg(1))
		if err != nil {
			return nil, err
		}
		opts := arg(2)
		var rel RelativeTo
		if !isUndefined(opts) {
			o, ok := opts.(*jsObject)
			if !ok {
				if !isObject(opts) {
					return nil, typeErrWithArg("Option must be object:", "relativeTo")
				}
				o = newObject()
			}
			if rel, err = h.relativeTo(o); err != nil {
				return nil, err
			}
		}
		c, err := one.Compare(two, rel)
		return float64(c), rustErr(err)
	}
	m["method:Duration.toString"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.Duration.prototype.toString"
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		digits, err := getFractionalSecondDigits(o)
		if err != nil {
			return nil, err
		}
		mode, err := getRoundingMode(o, method, Trunc)
		if err != nil {
			return nil, err
		}
		smallest, err := getUnitOption(o, "smallestUnit", method, false)
		if err != nil {
			return nil, err
		}
		if err := validateUnit(smallest, groupTime, NoUnit); err != nil {
			return nil, err
		}
		s, err := this.(Duration).String(ToStringRoundingOptions{Precision: digits, SmallestUnit: smallest, RoundingMode: mode})
		return s, rustErr(err)
	}
}

// relativeTo is GetTemporalRelativeToOption over an options object.
func (h *harness) relativeTo(o *jsObject) (RelativeTo, error) {
	v := o.get("relativeTo")
	switch x := v.(type) {
	case jsUndefined:
		return RelativeTo{}, nil
	case ZonedDateTime:
		return RelativeTo{Zoned: &x}, nil
	case PlainDate:
		return RelativeTo{Date: &x}, nil
	case PlainDateTime:
		d, err := h.plainDateFromTemporal(x.ToPlainDate(), nil)
		if err != nil {
			return RelativeTo{}, err
		}
		return RelativeTo{Date: &d}, nil
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return RelativeTo{}, err
		}
		rel, err := h.data.ParseRelativeTo(b)
		return rel, rustErr(err)
	}
	if !isObject(v) {
		return RelativeTo{}, typeErr("relativeTo must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return RelativeTo{}, err
	}
	f, err := h.prepareFields(cal, v, fieldsAllDate|fieldsTime|fieldsOffset|fieldsTimeZone, requireNone)
	if err != nil {
		return RelativeTo{}, err
	}
	p, err := f.regulateZoned(Constrain)
	if err != nil {
		return RelativeTo{}, err
	}
	if f.timeZone == nil {
		d, err := h.plainDateFromFields(p.Date, cal, Constrain, true)
		if err != nil {
			return RelativeTo{}, err
		}
		return RelativeTo{Date: &d}, nil
	}
	z, err := h.zones.ZonedDateTimeFromFields(p, f.timeZone, cal, Constrain, Compatible, OffsetReject)
	if err != nil {
		return RelativeTo{}, rustErr(err)
	}
	return RelativeTo{Zoned: &z}, nil
}
