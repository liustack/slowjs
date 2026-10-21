//go:build windows

package intl

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// The zone a Windows machine is set to, as ICU's uprv_detectWindowsTimeZone
// (wintz.cpp) finds it, which is what Node reports there where TZ is not
// set. ICU on Windows does not read TZ; Node does (see nodeTZ).

// nodeTZ is TZ, where it is not empty, which Node on Windows sets ICU's
// default zone from when it starts (InitializeOncePerProcess).
func nodeTZ() (string, bool) {
	tz := os.Getenv("TZ")
	return tz, tz != ""
}

var (
	kernel32                          = syscall.NewLazyDLL("kernel32.dll")
	procGetDynamicTimeZoneInfo        = kernel32.NewProc("GetDynamicTimeZoneInformation")
	procGetUserGeoID                  = kernel32.NewProc("GetUserGeoID")
	procGetGeoInfoW                   = kernel32.NewProc("GetGeoInfoW")
	timeZoneIDInvalid          uint32 = 0xFFFFFFFF
)

// systemTime is SYSTEMTIME, compared only as bytes.
type systemTime [8]uint16

// dynamicTimeZoneInformation is DYNAMIC_TIME_ZONE_INFORMATION.
type dynamicTimeZoneInformation struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                systemTime
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                systemTime
	DaylightBias                int32
	TimeZoneKeyName             [128]uint16
	DynamicDaylightTimeDisabled uint8
}

// regTZI is REG_TZI_FORMAT, a zone's TZI value in the registry.
type regTZI struct {
	Bias, StandardBias, DaylightBias int32
	StandardDate, DaylightDate       systemTime
}

// hostZone is uprv_tzname(0) and the raw offset, in seconds east, that
// uprv_timezone gives: the zone's bias.
func hostZone(src Source) (string, int) {
	var tzi dynamicTimeZoneInformation
	if r, _, _ := procGetDynamicTimeZoneInfo.Call(uintptr(unsafe.Pointer(&tzi))); uint32(r) == timeZoneIDInvalid {
		return "", 0
	}
	raw := -int(tzi.Bias) * 60
	// Daylight saving turned off in the Control Panel is Etc/GMT at the
	// offset, as the Control Panel itself decides it is off.
	var zero systemTime
	hasKey := tzi.TimeZoneKeyName[0] != 0
	if tzi.DynamicDaylightTimeDisabled != 0 && tzi.StandardDate == tzi.DaylightDate &&
		(hasKey && tzi.StandardDate == zero || !hasKey && tzi.StandardDate != zero) {
		if tzi.Bias == 0 {
			return "Etc/UTC", raw
		}
		if tzi.Bias%60 == 0 {
			return fmt.Sprintf("Etc/GMT%+d", tzi.Bias/60), raw
		}
	}
	name := syscall.UTF16ToString(tzi.TimeZoneKeyName[:])
	if name == "" {
		// A remote session may give the localized standard name alone, to be
		// found among the zones in the registry.
		if name = registryZoneByStandardName(&tzi); name == "" {
			return "", raw
		}
	}
	id, err := windowsZone(src, name, userRegion())
	if err != nil {
		return "", raw
	}
	return id, raw
}

// userRegion is the ISO code of the user's region, GetUserGeoID's nation
// by GetGeoInfoW; empty when Windows has none.
func userRegion() string {
	const geoClassNation, geoISO2 = 16, 4
	geo, _, _ := procGetUserGeoID.Call(geoClassNation)
	var buf [3]uint16
	n, _, _ := procGetGeoInfoW.Call(geo, geoISO2, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}

// registryZoneByStandardName is ICU's search of the registry's zones for
// the one whose localized standard name, bias and dates are the machine's.
func registryZoneByStandardName(tzi *dynamicTimeZoneInformation) string {
	path, _ := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Windows NT\CurrentVersion\Time Zones`)
	var all syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, path, 0, syscall.KEY_READ, &all) != nil {
		return ""
	}
	defer syscall.RegCloseKey(all)
	var count uint32
	if syscall.RegQueryInfoKey(all, nil, nil, nil, &count, nil, nil, nil, nil, nil, nil, nil) != nil {
		return ""
	}
	std, _ := syscall.UTF16PtrFromString("Std")
	tziName, _ := syscall.UTF16PtrFromString("TZI")
	for i := uint32(0); i < count; i++ {
		var keyName [256]uint16
		size := uint32(len(keyName))
		if syscall.RegEnumKeyEx(all, i, &keyName[0], &size, nil, nil, nil, nil) != nil {
			return ""
		}
		var key syscall.Handle
		if syscall.RegOpenKeyEx(all, &keyName[0], 0, syscall.KEY_READ, &key) != nil {
			return ""
		}
		var stdName [256]uint16
		n := uint32(len(stdName) * 2)
		var kind uint32
		if syscall.RegQueryValueEx(key, std, nil, &kind, (*byte)(unsafe.Pointer(&stdName[0])), &n) != nil ||
			kind != syscall.REG_SZ {
			syscall.RegCloseKey(key)
			return ""
		}
		if syscall.UTF16ToString(stdName[:]) == syscall.UTF16ToString(tzi.StandardName[:]) {
			var reg regTZI
			n = uint32(unsafe.Sizeof(reg))
			if syscall.RegQueryValueEx(key, tziName, nil, &kind, (*byte)(unsafe.Pointer(&reg)), &n) == nil &&
				reg.Bias == tzi.Bias && reg.StandardDate == tzi.StandardDate && reg.DaylightDate == tzi.DaylightDate {
				syscall.RegCloseKey(key)
				return syscall.UTF16ToString(keyName[:size])
			}
		}
		syscall.RegCloseKey(key)
	}
	return ""
}

// hostAbbreviations is not read on Windows: ICU names the zone Windows is
// set to, and where it cannot, the host is Etc/Unknown.
func hostAbbreviations() (hostAbbreviation, bool) { return hostAbbreviation{}, false }
