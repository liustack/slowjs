//go:build !windows

package intl

import (
	"os"
	"strings"
)

// hostLocaleID is uprv_getDefaultLocaleID on POSIX: the locale of
// LC_MESSAGES, as the environment gives it, "de_DE.UTF-8@euro" being
// de_DE_EURO's ID "de_DE_euro".
func hostLocaleID() string {
	id, ok := os.LookupEnv("LC_ALL")
	if !ok {
		if id, ok = os.LookupEnv("LC_MESSAGES"); !ok {
			id, ok = os.LookupEnv("LANG")
		}
	}
	if !ok || id == "C" || id == "POSIX" {
		return "en_US_POSIX"
	}
	corrected := id
	if i := strings.IndexByte(corrected, '.'); i >= 0 {
		corrected = corrected[:i]
	}
	if i := strings.IndexByte(corrected, '@'); i >= 0 {
		corrected = corrected[:i]
	}
	if corrected == "C" || corrected == "POSIX" {
		corrected = "en_US_POSIX"
	}
	if i := strings.LastIndexByte(id, '@'); i >= 0 {
		variant := id[i+1:]
		if variant == "nynorsk" {
			variant = "NY"
		}
		if strings.IndexByte(corrected, '_') < 0 {
			corrected += "__"
		} else {
			corrected += "_"
		}
		if j := strings.IndexByte(variant, '.'); j >= 0 {
			variant = variant[:j]
		}
		corrected += variant
	}
	return corrected
}
