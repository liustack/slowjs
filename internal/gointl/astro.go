package intl

import "math"

// The parts of ICU 78's CalendarAstronomer (astro.cpp) the lunar calendars
// reckon with: the longitudes of the sun and the moon from Duffett-Smith's
// "Practical Astronomy with your Calculator", and the search for the moment
// one reaches a given angle.
//
// A calendar that turns on the sign of an angle at midnight turns on the
// last bit of these sums, so they are written to round as ICU's do: each
// constant is the double ICU's macro makes, operation by operation, and
// each product is converted before it is added so that no platform fuses
// the two. What can still differ is the C library's sine against Go's, by
// a unit in the last place, which moves a date only when the moon is within
// a hair of new at midnight.

// astroPI is CalendarAstronomer::PI, and the other constants are its macros.
const (
	astroPI            = 3.14159265358979323846
	astroSynodicMonth  = 29.530588853
	astroTropicalYear  = 365.242191
	astroJulianEpochMS = -210866760000000.0
	astroJDEpoch       = 2447891.5
	astroSunE          = 0.016713
	astroDayMS         = 86400000.0
	astroMinuteMS      = 60000.0
)

// rad is "(x * CalendarAstronomer::PI/180)", rounded as C++ rounds it.
func rad(x float64) float64 {
	pi := float64(astroPI)
	return x * pi / 180
}

// pi2 is CalendarAstronomer_PI2, PI*2.0.
func pi2() float64 {
	pi := float64(astroPI)
	return pi * 2.0
}

// norm2PI brings an angle into [0, 2π).
func norm2PI(angle float64) float64 {
	r := pi2()
	return angle - float64(r*math.Floor(angle/r))
}

// normPI brings an angle into [-π, π).
func normPI(angle float64) float64 {
	pi := float64(astroPI)
	return norm2PI(angle+pi) - pi
}

// astronomer is a CalendarAstronomer set to one moment, in milliseconds
// since 1970, with what it has worked out for that moment kept as ICU keeps
// it.
type astronomer struct {
	time float64

	haveSun, haveMoon       bool
	sunLong, meanAnomalySun float64
	moonEclipLong           float64
}

func newAstronomer(ms float64) *astronomer { return &astronomer{time: ms} }

func (a *astronomer) setTime(ms float64) { *a = astronomer{time: ms} }

// julianDay is the moment as a fractional Julian day.
func (a *astronomer) julianDay() float64 {
	return (a.time - astroJulianEpochMS) / astroDayMS
}

// trueAnomaly solves Kepler's equation, as astro.cpp's does, to 1e-5.
func trueAnomaly(meanAnomaly, e float64) float64 {
	E := meanAnomaly
	for {
		delta := E - float64(e*math.Sin(E)) - meanAnomaly
		E = E - delta/(1-float64(e*math.Cos(E)))
		if math.Abs(delta) <= 1e-5 {
			break
		}
	}
	return 2.0 * math.Atan(math.Tan(E/2)*math.Sqrt((1+e)/(1-e)))
}

// sunLongitudeAt is getSunLongitude(jDay, ...): the sun's ecliptic
// longitude and mean anomaly.
func sunLongitudeAt(jd float64) (longitude, meanAnomaly float64) {
	day := jd - astroJDEpoch
	epochAngle := norm2PI(float64(pi2() / astroTropicalYear * day))
	meanAnomaly = norm2PI(epochAngle + rad(279.403303) - rad(282.768422))
	longitude = norm2PI(trueAnomaly(meanAnomaly, astroSunE) + rad(282.768422))
	return longitude, meanAnomaly
}

// sunLongitude is getSunLongitude().
func (a *astronomer) sunLongitude() float64 {
	if !a.haveSun {
		a.sunLong, a.meanAnomalySun = sunLongitudeAt(a.julianDay())
		a.haveSun = true
	}
	return a.sunLong
}

// moonPosition is getMoonPosition, as far as the moon's ecliptic longitude:
// the latitude and the equatorial coordinates it goes on to are not needed.
func (a *astronomer) moonPosition() {
	if a.haveMoon {
		return
	}
	a.sunLongitude()
	sun, anomalySun := a.sunLong, a.meanAnomalySun
	day := a.julianDay() - astroJDEpoch

	meanLongitude := norm2PI(float64(rad(13.1763966)*day) + rad(318.351648))
	meanAnomalyMoon := norm2PI(meanLongitude - float64(rad(0.1114041)*day) - rad(36.340410))

	evection := float64(rad(1.2739) * math.Sin(2*(meanLongitude-sun)-meanAnomalyMoon))
	annual := float64(rad(0.1858) * math.Sin(anomalySun))
	a3 := float64(rad(0.3700) * math.Sin(anomalySun))
	meanAnomalyMoon += evection - annual - a3

	center := float64(rad(6.2886) * math.Sin(meanAnomalyMoon))
	a4 := float64(rad(0.2140) * math.Sin(2*meanAnomalyMoon))
	moonLongitude := meanLongitude + evection + center - annual + a4

	variation := float64(rad(0.6583) * math.Sin(2*(moonLongitude-sun)))
	moonLongitude += variation

	nodeLongitude := norm2PI(rad(318.510107) - float64(rad(0.0529539)*day))
	nodeLongitude -= float64(rad(0.16) * math.Sin(anomalySun))
	y := math.Sin(moonLongitude - nodeLongitude)
	x := math.Cos(moonLongitude - nodeLongitude)
	a.moonEclipLong = math.Atan2(float64(y*math.Cos(rad(5.145366))), x) + nodeLongitude
	a.haveMoon = true
}

// moonAge is getMoonAge: the angle from the sun to the moon along the
// ecliptic, in radians, zero at the new moon.
func (a *astronomer) moonAge() float64 {
	a.moonPosition()
	return norm2PI(a.moonEclipLong - a.sunLong)
}

// timeOfAngle is CalendarAstronomer::timeOfAngle: the next or previous
// moment an angle, the sun's longitude or the moon's age, reaches a value,
// found by secant steps from an estimate made with the average period.
func (a *astronomer) timeOfAngle(angleAt func(*astronomer) float64, desired, periodDays, epsilon float64, next bool) float64 {
	for {
		lastAngle := angleAt(a)
		deltaAngle := norm2PI(desired - lastAngle)
		back := 0.0
		if !next {
			back = -pi2()
		}
		deltaT := (deltaAngle + back) * float64(periodDays*astroDayMS) / pi2()
		lastDeltaT := deltaT
		startTime := a.time
		a.setTime(a.time + math.Ceil(deltaT))
		restart := false
		for {
			angle := angleAt(a)
			factor := math.Abs(deltaT / normPI(angle-lastAngle))
			deltaT = normPI(desired-angle) * factor
			if math.Abs(deltaT) > math.Abs(lastDeltaT) {
				// ICU's HACK: a diverging search starts again an eighth of a
				// period further on.
				delta := math.Ceil(float64(periodDays*astroDayMS) / 8.0)
				if !next {
					delta = -delta
				}
				a.setTime(startTime + delta)
				restart = true
				break
			}
			lastDeltaT = deltaT
			lastAngle = angle
			a.setTime(a.time + math.Ceil(deltaT))
			if math.Abs(deltaT) <= epsilon {
				break
			}
		}
		if !restart {
			return a.time
		}
	}
}

// sunTime is getSunTime: when the sun's longitude next (or last) reaches
// desired.
func (a *astronomer) sunTime(desired float64, next bool) float64 {
	return a.timeOfAngle((*astronomer).sunLongitude, desired, astroTropicalYear, astroMinuteMS, next)
}

// moonTime is getMoonTime: when the moon's age next (or last) reaches
// desired.
func (a *astronomer) moonTime(desired float64, next bool) float64 {
	return a.timeOfAngle((*astronomer).moonAge, desired, astroSynodicMonth, astroMinuteMS, next)
}
