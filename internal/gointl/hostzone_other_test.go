//go:build gointl_all && !windows

package intl

import "testing"

// TestValidOlsonID is ICU's isValidOlsonID: a POSIX rule in TZ is not a
// zone's name, though the four such names the tz database keeps are.
func TestValidOlsonID(t *testing.T) {
	for id, want := range map[string]bool{
		"America/New_York":         true,
		"Etc/GMT+11":               true,
		"UTC":                      true,
		"":                         true,
		"EST5EDT":                  true,
		"PST8PDT":                  true,
		"AST4ADT":                  false,
		"CST6CDT5,J129,J131/19:30": false,
		"Etc/GMT+123":              false,
	} {
		if got := validOlsonID(id); got != want {
			t.Errorf("validOlsonID(%q) = %v, want %v", id, got, want)
		}
	}
	for in, want := range map[string]string{
		"posix/Europe/Paris": "Europe/Paris",
		"right/UTC":          "UTC",
		"Europe/Paris":       "Europe/Paris",
	} {
		if got := skipZoneIDPrefix(in); got != want {
			t.Errorf("skipZoneIDPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
