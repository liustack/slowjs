//go:build !windows

package intl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The zone a machine other than Windows is set to, as ICU's uprv_tzname
// (putil.cpp) finds it on Linux, macOS and the BSDs, which is what Node
// reports there: TZ, where it names a zone, then the zone /etc/localtime
// links to, then the zone file it is a copy of.
//
// Where none of them names a zone, ICU guesses from the C library's two
// abbreviations, which hostAbbreviations reads as the C library would from
// the zone Go's time package loads (see hostabbrev.go).

const (
	tzDefault  = "/etc/localtime"
	tzZoneInfo = "/usr/share/zoneinfo/"
)

// nodeTZ is Node's reading of TZ, which it does itself only on Windows;
// here ICU reads it (hostZoneName).
func nodeTZ() (string, bool) { return "", false }

// hostZone is uprv_tzname(0), and the host's standard offset, in seconds
// east, which is the C library's timezone.
func hostZone(src Source) (string, int) {
	return hostZoneName(), hostRawOffset()
}

func hostZoneName() string {
	if tz, ok := os.LookupEnv("TZ"); ok && validOlsonID(tz) {
		// A colon has tzset read the rest as a file under zoneinfo.
		return skipZoneIDPrefix(strings.TrimPrefix(tz, ":"))
	}
	target, err := filepath.EvalSymlinks(tzDefault)
	if err == nil && target != tzDefault {
		// The link's target, under a directory named zoneinfo: macOS's
		// resolves to zoneinfo.default, and some point at posixrules, where
		// the link as written is the one to read.
		tail := strings.Index(target, "/zoneinfo/")
		if tail < 0 || target[tail+len("/zoneinfo/"):] == "posixrules" {
			if link, err := os.Readlink(tzDefault); err == nil {
				target = link
				tail = strings.Index(target, "/zoneinfo/")
			}
		}
		if tail >= 0 {
			if id := skipZoneIDPrefix(target[tail+len("/zoneinfo/"):]); validOlsonID(id) {
				return id
			}
		}
		// A link to no zone's name leaves ICU nothing but the
		// abbreviations.
		return ""
	}
	want, err := os.ReadFile(tzDefault)
	if err != nil {
		return ""
	}
	if id := searchForTZFile(tzZoneInfo, want); validOlsonID(id) {
		return id
	}
	return ""
}

// searchForTZFile is ICU's search of the zone files for the first that is
// a copy of /etc/localtime, in the order the directories list them.
func searchForTZFile(dir string, want []byte) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if name == "posixrules" || name == "localtime" {
			continue
		}
		path := dir + name
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if id := searchForTZFile(path+"/", want); id != "" {
				return id
			}
			continue
		}
		if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, want) {
			return skipZoneIDPrefix(path[len(tzZoneInfo):])
		}
	}
	return ""
}

// validOlsonID is ICU's isValidOlsonID: not a POSIX rule such as
// "CST6CDT5,J129,J131/19:30", though "EST5EDT" and its like are zones.
func validOlsonID(id string) bool {
	i := 0
	for i < len(id) && (id[i] < '0' || id[i] > '9') && id[i] != ',' {
		i++
	}
	for max := i + 2; i < len(id) && id[i] >= '0' && id[i] <= '9' && i < max; i++ {
	}
	return i == len(id) || id == "PST8PDT" || id == "MST7MDT" || id == "CST6CDT" || id == "EST5EDT"
}

// skipZoneIDPrefix drops the "posix/" and "right/" that copies of the zone
// files live under.
func skipZoneIDPrefix(id string) string {
	if strings.HasPrefix(id, "posix/") || strings.HasPrefix(id, "right/") {
		return id[len("posix/"):]
	}
	return id
}

// hostRawOffset is the host's standard offset as the C library has it, in
// seconds east: the offset now, or, in summer time, the one before it.
func hostRawOffset() int {
	now := time.Now()
	_, offset := now.Zone()
	if now.IsDST() {
		if start, _ := now.ZoneBounds(); !start.IsZero() {
			_, offset = start.Add(-time.Second).Zone()
		}
	}
	return offset
}

// hostAbbreviations is what ICU reads of the C library's zone, which on
// Linux, macOS and the BSDs is /etc/localtime's, or TZ's, as Go's time
// package loads it too.
func hostAbbreviations() (hostAbbreviation, bool) { return abbreviationsOf(time.Local), true }
