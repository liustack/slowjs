package date

import "math"

// A Date is a Date object as V8 keeps one (JSDate): its time value, and the
// local year to second read through the offset cache whenever the value is
// set, which the getters of those fields answer from afterwards.
//
// Reading local time when the value is set asks the cache, and what the
// cache answers next depends on it (see cache.go), so an engine that is to
// answer as Node does makes and sets its Date objects here: new Date(t)
// followed by getTimezoneOffset() is two questions to the cache, not one.
//
// A Date is a plain value; it is not safe to set from one goroutine while
// another reads it.
type Date struct {
	value float64
	// env is V8's cache stamp: the Environment local was read in. A Date
	// read in another one reads its local fields again.
	env   *Environment
	local Fields
}

// NewDate is a Date object made with a time value, JSDate::New: the value
// clipped, and its local time read.
func (e *Environment) NewDate(t float64) *Date {
	d := new(Date)
	e.SetTime(d, t)
	return d
}

// SetTime sets a Date's time value, clipped, as V8 sets it (SetDateValue)
// from a value in UTC: it is setTime, and each of the setUTC methods once it
// has its value. It returns the value set.
func (e *Environment) SetTime(d *Date, t float64) float64 {
	d.value = TimeClip(t)
	d.env = e
	if !math.IsNaN(d.value) {
		d.local = BreakDown(e.LocalTime(d.value))
	}
	return d.value
}

// SetLocalTime sets a Date's time value from a local time, as V8 sets it
// (SetLocalDateValue): the local time made a time value by UTC, and set. It
// is each of the set methods that take local fields, once it has its value.
// It returns the value set.
func (e *Environment) SetLocalTime(d *Date, local float64) float64 {
	return e.SetTime(d, e.UTC(local))
}

// Value is the Date's time value: getTime and valueOf.
func (d *Date) Value() float64 { return d.value }

// Local is the Date's local fields as V8's getters read them: the year to
// the second and the weekday as they were read when the value was set, read
// again only where it was set in another Environment. Millisecond is the
// time value's; V8 keeps no local millisecond, and its getMilliseconds reads
// LocalTime again. It is false for an invalid date.
func (e *Environment) Local(d *Date) (Fields, bool) {
	if math.IsNaN(d.value) {
		return Fields{}, false
	}
	if d.env != e {
		d.env = e
		d.local = BreakDown(e.LocalTime(d.value))
	}
	f := d.local
	f.Millisecond = BreakDown(d.value).Millisecond
	return f, true
}
