package temporal

// ISO 8601 and RFC 9557 strings, parsed as ICU4X's ixdtf crate 0.6.4
// parses them, over the bytes temporal_rs hands it.

// ixdtfError is ixdtf's ParseError, by its message.
type ixdtfError string

const (
	errImplAssert                 ixdtfError = "Implementation error: this error must not throw."
	errAbruptEnd                  ixdtfError = "Parsing ended abruptly."
	errInvalidEnd                 ixdtfError = "Unexpected character found after parsing was completed."
	errInvalidMonthRange          ixdtfError = "Parsed month value not in a valid range."
	errInvalidDayRange            ixdtfError = "Parsed day value not in a valid range."
	errDateYear                   ixdtfError = "Invalid character while parsing year value."
	errDateExtendedYear           ixdtfError = "Invalid character while parsing extended year value."
	errDateMonth                  ixdtfError = "Invalid character while parsing month value."
	errDateDay                    ixdtfError = "Invalid character while parsing day value."
	errTimeHour                   ixdtfError = "Invalid character while parsing hour value."
	errTimeMinuteSecond           ixdtfError = "Invalid character while parsing minute/second value in (0, 59] range."
	errTimeSecond                 ixdtfError = "Invalid character while parsing second value in (0, 60] range."
	errFractionPart               ixdtfError = "Invalid character while parsing fraction part value."
	errDateSeparator              ixdtfError = "Invalid character while parsing date separator."
	errTimeSeparator              ixdtfError = "Invalid character while parsing time separator."
	errAnnotationOpen             ixdtfError = "Invalid annotation open character."
	errAnnotationClose            ixdtfError = "Invalid annotation close character."
	errAnnotationChar             ixdtfError = "Invalid annotation character."
	errAnnotationKeyValueSep      ixdtfError = "Invalid annotation key-value separator character."
	errAnnotationKeyLeadingChar   ixdtfError = "Invalid annotation key leading character."
	errAnnotationKeyChar          ixdtfError = "Invalid annotation key character."
	errAnnotationValueCharPostHyp ixdtfError = "Expected annotation value character must exist after hyphen."
	errAnnotationValueChar        ixdtfError = "Invalid annotation value character."
	errInvalidMinutePrecision     ixdtfError = "Offset must be minute precision"
	errCriticalDuplicateCalendar  ixdtfError = "Duplicate calendars cannot be provided when one is critical."
	errUnrecognizedCritical       ixdtfError = "Unrecognized annoation is marked as critical."
	errTzLeadingChar              ixdtfError = "Invalid time zone leading character."
	errIanaCharPostSeparator      ixdtfError = "Expected time zone character after '/'."
	errIanaChar                   ixdtfError = "Invalid IANA time zone character after '/'."
	errUtcTimeSeparator           ixdtfError = "Invalid time zone character after '/'."
	errOffsetNeedsSign            ixdtfError = "UTC offset needs a sign"
	errMonthDayHyphen             ixdtfError = "MonthDay must begin with a month or '--'"
	errDurationDesignator         ixdtfError = "Invalid duration designator."
	errDurationValueExceededRange ixdtfError = "Provided Duration field value exceeds supported range."
	errDateDurationPartOrder      ixdtfError = "Invalid date duration part order."
	errTimeDurationPartOrder      ixdtfError = "Invalid time duration part order."
	errTimeDurationDesignator     ixdtfError = "Invalid time duration designator."
	errAmbiguousTimeMonthDay      ixdtfError = "Time is ambiguous with MonthDay"
	errAmbiguousTimeYearMonth     ixdtfError = "Time is ambiguous with YearMonth"
	errInvalidMonthDay            ixdtfError = "MonthDay was not valid."
)

func (e ixdtfError) Error() string { return string(e) }

// asTemporal is the RangeError temporal_rs makes of a parse error.
func (e ixdtfError) asTemporal() error { return rangeError("%s", string(e)) }

type cursor struct {
	pos int
	src []byte
}

// at is the byte n on, and false past the end.
func (c *cursor) at(n int) (byte, bool) {
	if c.pos+n < len(c.src) {
		return c.src[c.pos+n], true
	}
	return 0, false
}

func (c *cursor) current() (byte, bool) { return c.at(0) }

func (c *cursor) checkOr(def bool, f func(byte) bool) bool {
	b, ok := c.current()
	if !ok {
		return def
	}
	return f(b)
}

func (c *cursor) next() (byte, bool) {
	b, ok := c.current()
	c.pos++
	return b, ok
}

// nextDigit is Cursor::next_digit: the next byte's value if a digit, false
// if it is not, an error at the end.
func (c *cursor) nextDigit() (byte, bool, error) {
	b, ok := c.next()
	if !ok {
		return 0, false, errAbruptEnd
	}
	if b >= '0' && b <= '9' {
		return b - '0', true, nil
	}
	return 0, false, nil
}

func (c *cursor) advanceIf(cond bool) {
	if cond {
		c.pos++
	}
}

func (c *cursor) close() error {
	if c.pos < len(c.src) {
		return errInvalidEnd
	}
	return nil
}

func isDigit(b byte) bool      { return b >= '0' && b <= '9' }
func isLower(b byte) bool      { return b >= 'a' && b <= 'z' }
func isAlpha(b byte) bool      { return isLower(b) || b >= 'A' && b <= 'Z' }
func isSign(b byte) bool       { return b == '+' || b == '-' }
func isKeyLeading(b byte) bool { return isLower(b) || b == '_' }
func isKeyChar(b byte) bool    { return isKeyLeading(b) || isDigit(b) || b == '-' }
func isTzLeading(b byte) bool  { return isAlpha(b) || b == '_' || b == '.' }
func isTzChar(b byte) bool     { return isTzLeading(b) || isDigit(b) || b == '-' || b == '+' }
func isTimeDesig(b byte) bool  { return b == 'T' || b == 't' }
func isDTSep(b byte) bool      { return isTimeDesig(b) || b == ' ' }
func isUTCDesig(b byte) bool   { return b == 'Z' || b == 'z' }
func isDecimalSep(b byte) bool { return b == '.' || b == ',' }
func isHyphen(b byte) bool     { return b == '-' }
func isColon(b byte) bool      { return b == ':' }
func isOpen(b byte) bool       { return b == '[' }
func isClose(b byte) bool      { return b == ']' }

// dateRecord is DateRecord.
type dateRecord struct {
	year       int32
	month, day uint8
}

// fraction is ixdtf's Fraction: the digits' count and value, the first 18.
type fraction struct {
	digits uint8
	value  uint64
}

// nanoseconds is Fraction::to_nanoseconds, false beyond nine digits.
func (f fraction) nanoseconds() (uint32, bool) {
	if f.digits > 9 {
		return 0, false
	}
	return pow10(9-int(f.digits)) * uint32(f.value), true
}

// timeRecord is TimeRecord.
type timeRecord struct {
	hour, minute, second uint8
	fraction             *fraction
}

// offsetRecord is UtcOffsetRecord: an offset to the minute, or to the
// second with a fraction.
type offsetRecord struct {
	negative     bool
	hour, minute uint8
	hasSecond    bool
	second       uint8
	fraction     *fraction
}

// tzRecord is TimeZoneRecord: a name, or an offset to the minute.
type tzRecord struct {
	name   []byte
	isName bool
	offset offsetRecord
}

// parseRecord is IxdtfParseRecord.
type parseRecord struct {
	date     *dateRecord
	time     *timeRecord
	z        bool // the offset is Z
	offset   *offsetRecord
	tz       *tzRecord
	calendar []byte
	hasCal   bool
}

// annotation is Annotation.
type annotation struct {
	critical   bool
	key, value []byte
}

// annotationHandler is the handler temporal_rs passes: it sees each
// annotation and keeps the ones it returns true for.
type annotationHandler func(annotation) bool

func parseAnnotatedDateTime(c *cursor, h annotationHandler) (parseRecord, error) {
	r, err := parseDateTimeRecord(c)
	if err != nil {
		return r, err
	}
	if !c.checkOr(false, isOpen) {
		return r, c.close()
	}
	if err := parseAnnotationSet(c, h, &r); err != nil {
		return r, err
	}
	return r, c.close()
}

func parseDateTimeRecord(c *cursor) (parseRecord, error) {
	var r parseRecord
	d, err := parseDate(c)
	if err != nil {
		return r, err
	}
	r.date = &d
	if !c.checkOr(false, isDTSep) {
		return r, nil
	}
	c.pos++
	t, err := parseTimeRecord(c)
	if err != nil {
		return r, err
	}
	r.time = &t
	if c.checkOr(false, func(b byte) bool { return isSign(b) || isUTCDesig(b) }) {
		if err := parseDateTimeOffset(c, &r); err != nil {
			return r, err
		}
	}
	return r, nil
}

func parseDate(c *cursor) (dateRecord, error) {
	year, err := parseDateYear(c)
	if err != nil {
		return dateRecord{}, err
	}
	b, ok := c.current()
	if !ok {
		return dateRecord{}, errAbruptEnd
	}
	hyphenated := isHyphen(b)
	c.advanceIf(hyphenated)
	month, err := parseDateMonth(c)
	if err != nil {
		return dateRecord{}, err
	}
	second := c.checkOr(false, isHyphen)
	if hyphenated != second {
		return dateRecord{}, errDateSeparator
	}
	c.advanceIf(second)
	day, err := parseDateDay(c)
	if err != nil {
		return dateRecord{}, err
	}
	if day < 1 || int(day) > gregorianMonthLength(int(year), int(month)) {
		return dateRecord{}, errInvalidDayRange
	}
	return dateRecord{year, month, day}, nil
}

func parseDateYear(c *cursor) (int32, error) {
	if c.checkOr(false, isSign) {
		b, _ := c.next()
		sign := int32(1)
		if b != '+' {
			sign = -1
		}
		var v int32
		for i := 0; i < 6; i++ {
			d, ok, err := c.nextDigit()
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, errDateExtendedYear
			}
			v = v*10 + int32(d)
		}
		if sign == -1 && v == 0 {
			return 0, errDateExtendedYear
		}
		return sign * v, nil
	}
	var v int32
	for i := 0; i < 4; i++ {
		d, ok, err := c.nextDigit()
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, errDateYear
		}
		v = v*10 + int32(d)
	}
	return v, nil
}

func twoDigits(c *cursor, bad ixdtfError) (uint8, error) {
	d1, ok, err := c.nextDigit()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, bad
	}
	d2, ok, err := c.nextDigit()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, bad
	}
	return d1*10 + d2, nil
}

func parseDateMonth(c *cursor) (uint8, error) {
	m, err := twoDigits(c, errDateMonth)
	if err != nil {
		return 0, err
	}
	if m < 1 || m > 12 {
		return 0, errInvalidMonthRange
	}
	return m, nil
}

func parseDateDay(c *cursor) (uint8, error) { return twoDigits(c, errDateDay) }

func parseAnnotatedYearMonth(c *cursor, h annotationHandler) (parseRecord, error) {
	var r parseRecord
	d, err := parseYearMonth(c)
	if err != nil {
		return r, err
	}
	r.date = &d
	if !c.checkOr(false, isOpen) {
		return r, c.close()
	}
	return r, parseAnnotationSet(c, h, &r)
}

func parseYearMonth(c *cursor) (dateRecord, error) {
	year, err := parseDateYear(c)
	if err != nil {
		return dateRecord{}, err
	}
	c.advanceIf(c.checkOr(false, isHyphen))
	month, err := parseDateMonth(c)
	if err != nil {
		return dateRecord{}, err
	}
	return dateRecord{year, month, 1}, nil
}

func parseAnnotatedMonthDay(c *cursor, h annotationHandler) (parseRecord, error) {
	var r parseRecord
	d, err := parseMonthDay(c)
	if err != nil {
		return r, err
	}
	r.date = &d
	if !c.checkOr(false, isOpen) {
		return r, c.close()
	}
	return r, parseAnnotationSet(c, h, &r)
}

func parseMonthDay(c *cursor) (dateRecord, error) {
	b, ok := c.current()
	if !ok {
		return dateRecord{}, errAbruptEnd
	}
	hyphenated := isHyphen(b)
	c.advanceIf(hyphenated)
	balanced := false
	if hyphenated {
		b, ok := c.current()
		if !ok {
			return dateRecord{}, errAbruptEnd
		}
		balanced = isHyphen(b)
	}
	c.advanceIf(balanced)
	if hyphenated && !balanced {
		return dateRecord{}, errMonthDayHyphen
	}
	month, err := parseDateMonth(c)
	if err != nil {
		return dateRecord{}, err
	}
	c.advanceIf(c.checkOr(false, isHyphen))
	day, err := parseDateDay(c)
	if err != nil {
		return dateRecord{}, err
	}
	switch {
	case (month == 2 || month == 4 || month == 6 || month == 9 || month == 11) && day >= 31,
		month == 2 && day == 30, day > 31:
		return dateRecord{}, errInvalidMonthDay
	}
	return dateRecord{0, month, day}, nil
}

func parseAnnotatedTime(c *cursor, h annotationHandler) (parseRecord, error) {
	var r parseRecord
	start := c.pos
	c.advanceIf(c.checkOr(false, isTimeDesig))
	t, err := parseTimeRecord(c)
	if err != nil {
		return r, err
	}
	r.time = &t
	if c.checkOr(false, func(b byte) bool { return isSign(b) || isUTCDesig(b) }) {
		if err := parseDateTimeOffset(c, &r); err != nil {
			return r, err
		}
	}
	if !c.checkOr(false, isOpen) {
		if err := c.close(); err != nil {
			return r, err
		}
		return r, checkTimeAmbiguity(c, start)
	}
	if err := checkTimeAmbiguity(c, start); err != nil {
		return r, err
	}
	if err := parseAnnotationSet(c, h, &r); err != nil {
		return r, err
	}
	return r, c.close()
}

func checkTimeAmbiguity(c *cursor, start int) error {
	current := c.pos
	c.pos = start
	if _, err := parseMonthDay(c); err == nil {
		return errAmbiguousTimeMonthDay
	}
	c.pos = start
	if _, err := parseYearMonth(c); err == nil {
		return errAmbiguousTimeYearMonth
	}
	c.pos = current
	return nil
}

func parseTimeRecord(c *cursor) (timeRecord, error) {
	hour, err := parseHour(c)
	if err != nil {
		return timeRecord{}, err
	}
	colonOrDigit := func(b byte) bool { return isColon(b) || isDigit(b) }
	if !c.checkOr(false, colonOrDigit) {
		return timeRecord{hour: hour}, nil
	}
	sep := c.checkOr(false, isColon)
	c.advanceIf(sep)
	minute, err := parseMinuteSecond(c, false)
	if err != nil {
		return timeRecord{}, err
	}
	if !c.checkOr(false, colonOrDigit) {
		return timeRecord{hour: hour, minute: minute}, nil
	}
	sep2 := c.checkOr(false, isColon)
	if sep != sep2 {
		return timeRecord{}, errTimeSeparator
	}
	c.advanceIf(sep2)
	second, err := parseMinuteSecond(c, true)
	if err != nil {
		return timeRecord{}, err
	}
	f, err := parseFraction(c)
	if err != nil {
		return timeRecord{}, err
	}
	return timeRecord{hour, minute, second, f}, nil
}

func parseHour(c *cursor) (uint8, error) {
	h, err := twoDigits(c, errTimeHour)
	if err != nil {
		return 0, err
	}
	if h > 23 {
		return 0, errTimeHour
	}
	return h, nil
}

func parseMinuteSecond(c *cursor, leapSecond bool) (uint8, error) {
	bad, max := errTimeMinuteSecond, uint8(59)
	if leapSecond {
		bad, max = errTimeSecond, 60
	}
	v, err := twoDigits(c, bad)
	if err != nil {
		return 0, err
	}
	if v > max {
		return 0, bad
	}
	return v, nil
}

func parseFraction(c *cursor) (*fraction, error) {
	if !c.checkOr(false, isDecimalSep) {
		return nil, nil
	}
	c.pos++
	var value uint64
	var digits uint8
	for c.checkOr(false, isDigit) {
		d, _, _ := c.nextDigit()
		if digits < 18 {
			value = value*10 + uint64(d)
		}
		if digits < 255 {
			digits++
		}
	}
	if digits == 0 {
		return nil, errFractionPart
	}
	return &fraction{digits, value}, nil
}

// parseDateTimeOffset is parse_date_time_utc_offset.
func parseDateTimeOffset(c *cursor, r *parseRecord) error {
	if c.checkOr(false, isUTCDesig) {
		c.pos++
		r.z = true
		return nil
	}
	o, err := parseUTCOffset(c)
	if err != nil {
		return err
	}
	r.offset = &o
	return nil
}

func parseUTCOffset(c *cursor) (offsetRecord, error) {
	o, separated, err := parseOffsetMinutePrecision(c)
	if err != nil {
		return o, err
	}
	if !c.checkOr(false, func(b byte) bool { return isDigit(b) || isColon(b) }) {
		return o, nil
	}
	colon, _ := c.current()
	if separated != isColon(colon) {
		return o, errUtcTimeSeparator
	}
	c.advanceIf(c.checkOr(false, isColon))
	s, err := parseMinuteSecond(c, false)
	if err != nil {
		return o, err
	}
	f, err := parseFraction(c)
	if err != nil {
		return o, err
	}
	o.hasSecond, o.second, o.fraction = true, s, f
	return o, nil
}

func parseOffsetMinutePrecision(c *cursor) (offsetRecord, bool, error) {
	b, ok := c.next()
	if !ok {
		return offsetRecord{}, false, errAbruptEnd
	}
	if !isSign(b) {
		return offsetRecord{}, false, errOffsetNeedsSign
	}
	neg := b == '-'
	hour, err := parseHour(c)
	if err != nil {
		return offsetRecord{}, false, err
	}
	if !c.checkOr(false, func(b byte) bool { return isDigit(b) || isColon(b) }) {
		return offsetRecord{negative: neg, hour: hour}, false, nil
	}
	sep := c.checkOr(false, isColon)
	c.advanceIf(sep)
	minute, err := parseMinuteSecond(c, false)
	if err != nil {
		return offsetRecord{}, false, err
	}
	return offsetRecord{negative: neg, hour: hour, minute: minute}, sep, nil
}

func parseOffsetMinutePrecisionStrict(c *cursor) (offsetRecord, error) {
	o, _, err := parseOffsetMinutePrecision(c)
	if err != nil {
		return o, err
	}
	if c.checkOr(false, func(b byte) bool { return isColon(b) || isDigit(b) }) {
		return o, errInvalidMinutePrecision
	}
	return o, nil
}

// parseTimeZone is parse_time_zone: a name or a minute offset.
func parseTimeZone(c *cursor) (tzRecord, error) {
	b, ok := c.current()
	if !ok {
		return tzRecord{}, errAbruptEnd
	}
	if isTzLeading(b) {
		name, err := parseTzName(c)
		return tzRecord{name: name, isName: true}, err
	}
	if isSign(b) {
		o, err := parseOffsetMinutePrecisionStrict(c)
		return tzRecord{offset: o}, err
	}
	return tzRecord{}, errTzLeadingChar
}

func parseTzName(c *cursor) ([]byte, error) {
	if !c.checkOr(false, isTzLeading) {
		return nil, errTzLeadingChar
	}
	start := c.pos
	for {
		b, ok := c.next()
		if !ok {
			break
		}
		if c.checkOr(true, isClose) {
			break
		}
		if b == '/' {
			if !c.checkOr(false, isTzChar) {
				return nil, errIanaCharPostSeparator
			}
			continue
		}
		if !isTzChar(b) {
			return nil, errIanaChar
		}
	}
	end := c.pos
	if end > len(c.src) {
		return nil, errImplAssert
	}
	return c.src[start:end], nil
}

// parseAmbiguousTzAnnotation is parse_ambiguous_tz_annotation: the zone
// annotation, if the first annotation is one.
func parseAmbiguousTzAnnotation(c *cursor) (*tzRecord, error) {
	peek := 1
	b, ok := c.at(peek)
	if !ok {
		return nil, errAbruptEnd
	}
	if b == '!' {
		peek++
	}
	lead, ok := c.at(peek)
	if !ok {
		return nil, errAbruptEnd
	}
	if isKeyLeading(lead) {
		for p := peek + 1; ; p++ {
			b, ok := c.at(p)
			if !ok {
				return nil, errAbruptEnd
			}
			if b == '=' {
				return nil, nil
			}
			if isClose(b) {
				tz, err := parseTzAnnotation(c)
				return &tz, err
			}
		}
	}
	tz, err := parseTzAnnotation(c)
	return &tz, err
}

func parseTzAnnotation(c *cursor) (tzRecord, error) {
	b, ok := c.next()
	if !ok {
		return tzRecord{}, errAnnotationOpen
	}
	if !isOpen(b) {
		return tzRecord{}, errAnnotationOpen
	}
	c.advanceIf(c.checkOr(false, func(b byte) bool { return b == '!' }))
	tz, err := parseTimeZone(c)
	if err != nil {
		return tz, err
	}
	b, ok = c.next()
	if !ok || !isClose(b) {
		return tz, errAnnotationClose
	}
	return tz, nil
}

// parseAnnotationSet is parse_annotation_set: an optional zone annotation,
// then the others, of which it keeps the calendar.
func parseAnnotationSet(c *cursor, h annotationHandler, r *parseRecord) error {
	tz, err := parseAmbiguousTzAnnotation(c)
	if err != nil {
		return err
	}
	r.tz = tz
	if !c.checkOr(false, isOpen) {
		return nil
	}
	var calendar *annotation
	for c.checkOr(false, isOpen) {
		a, err := parseKVAnnotation(c)
		if err != nil {
			return err
		}
		if !h(a) {
			continue
		}
		if string(a.key) == "u-ca" {
			switch {
			case calendar == nil:
				calendar = &a
			case string(calendar.value) != string(a.value) && (calendar.critical || a.critical):
				return errCriticalDuplicateCalendar
			}
			continue
		}
		if a.critical {
			return errUnrecognizedCritical
		}
	}
	if calendar != nil {
		r.calendar, r.hasCal = calendar.value, true
	}
	return nil
}

func parseKVAnnotation(c *cursor) (annotation, error) {
	b, ok := c.next()
	if !ok || !isOpen(b) {
		return annotation{}, errAnnotationOpen
	}
	critical := c.checkOr(false, func(b byte) bool { return b == '!' })
	c.advanceIf(critical)
	key, err := parseAnnotationKey(c)
	if err != nil {
		return annotation{}, err
	}
	b, ok = c.next()
	if !ok {
		return annotation{}, errAnnotationKeyValueSep
	}
	if b != '=' {
		return annotation{}, errAnnotationKeyValueSep
	}
	value, err := parseAnnotationValue(c)
	if err != nil {
		return annotation{}, err
	}
	b, ok = c.next()
	if !ok || !isClose(b) {
		return annotation{}, errAnnotationClose
	}
	return annotation{critical, key, value}, nil
}

func parseAnnotationKey(c *cursor) ([]byte, error) {
	start := c.pos
	b, ok := c.next()
	if !ok || !isKeyLeading(b) {
		return nil, errAnnotationKeyLeadingChar
	}
	for {
		b, ok := c.next()
		if !ok {
			return nil, errAnnotationChar
		}
		if c.checkOr(false, func(b byte) bool { return b == '=' }) {
			return c.src[start:c.pos], nil
		}
		if !isKeyChar(b) {
			return nil, errAnnotationKeyChar
		}
	}
}

func parseAnnotationValue(c *cursor) ([]byte, error) {
	start := c.pos
	c.pos++
	for {
		b, ok := c.next()
		if !ok {
			return nil, errAnnotationValueChar
		}
		if c.checkOr(false, isClose) {
			if c.pos > len(c.src) {
				return nil, errImplAssert
			}
			return c.src[start:c.pos], nil
		}
		if isHyphen(b) {
			p, ok := c.at(1)
			if !ok || !(isDigit(p) || isAlpha(p)) {
				return nil, errAnnotationValueCharPostHyp
			}
			c.pos++
			continue
		}
		if !(isDigit(b) || isAlpha(b)) {
			return nil, errAnnotationValueChar
		}
	}
}

// durationRecord is DurationParseRecord.
type durationRecord struct {
	negative                bool
	hasDate                 bool
	years, months, weeks    uint32
	days                    uint64
	timeUnit                int // 0 none, 1 hours, 2 minutes, 3 seconds
	hours, minutes, seconds uint64
	fraction                *fraction
}

func parseDuration(c *cursor) (durationRecord, error) {
	var r durationRecord
	b, ok := c.current()
	if !ok {
		return r, errAbruptEnd
	}
	if isSign(b) {
		b, _ := c.next()
		r.negative = b != '+'
	}
	b, ok = c.next()
	if !ok {
		return r, errAbruptEnd
	}
	if b != 'P' && b != 'p' {
		return r, errDurationDesignator
	}
	b, ok = c.current()
	if !ok {
		return r, errAbruptEnd
	}
	if !isTimeDesig(b) {
		r.hasDate = true
		if err := parseDateDuration(c, &r); err != nil {
			return r, err
		}
	}
	if err := parseTimeDuration(c, &r); err != nil {
		return r, err
	}
	return r, c.close()
}

// digitsValue reads digits into a u64, an error on overflow.
func digitsValue(c *cursor) (uint64, error) {
	var v uint64
	for c.checkOr(false, isDigit) {
		d, _, err := c.nextDigit()
		if err != nil {
			return 0, err
		}
		if v > (1<<64-1-uint64(d))/10 {
			return 0, errDurationValueExceededRange
		}
		v = v*10 + uint64(d)
	}
	return v, nil
}

func parseDateDuration(c *cursor, r *durationRecord) error {
	prev := 0
	for c.checkOr(false, isDigit) {
		v, err := digitsValue(c)
		if err != nil {
			return err
		}
		b, ok := c.next()
		if !ok {
			return errAbruptEnd
		}
		to32 := func() (uint32, error) {
			if v > 1<<32-1 {
				return 0, errDurationValueExceededRange
			}
			return uint32(v), nil
		}
		switch {
		case b == 'Y' || b == 'y':
			if prev > 1 {
				return errDateDurationPartOrder
			}
			if r.years, err = to32(); err != nil {
				return err
			}
			prev = 1
		case b == 'M' || b == 'm':
			if prev > 2 {
				return errDateDurationPartOrder
			}
			if r.months, err = to32(); err != nil {
				return err
			}
			prev = 2
		case b == 'W' || b == 'w':
			if prev > 3 {
				return errDateDurationPartOrder
			}
			if r.weeks, err = to32(); err != nil {
				return err
			}
			prev = 3
		case b == 'D' || b == 'd':
			if prev > 4 {
				return errDateDurationPartOrder
			}
			r.days = v
			prev = 4
		default:
			return errAbruptEnd
		}
	}
	return nil
}

func parseTimeDuration(c *cursor, r *durationRecord) error {
	if !c.checkOr(false, isTimeDesig) {
		return nil
	}
	c.pos++
	if !c.checkOr(false, isDigit) {
		return errTimeDurationDesignator
	}
	prev := 0
	for c.checkOr(false, isDigit) {
		v, err := digitsValue(c)
		if err != nil {
			return err
		}
		f, err := parseFraction(c)
		if err != nil {
			return err
		}
		b, ok := c.next()
		if !ok {
			return errAbruptEnd
		}
		switch {
		case b == 'H' || b == 'h':
			if prev > 1 {
				return errTimeDurationPartOrder
			}
			r.hours = v
			prev = 1
		case b == 'M' || b == 'm':
			if prev > 2 {
				return errTimeDurationPartOrder
			}
			r.minutes = v
			prev = 2
		case b == 'S' || b == 's':
			if prev > 3 {
				return errTimeDurationPartOrder
			}
			r.seconds = v
			prev = 3
		default:
			return errAbruptEnd
		}
		if f != nil {
			r.fraction = f
		}
		if f != nil {
			if !c.checkOr(true, func(b byte) bool { return !isDigit(b) }) {
				return errInvalidEnd
			}
			break
		}
	}
	if prev == 0 {
		return errAbruptEnd
	}
	r.timeUnit = prev
	return nil
}
