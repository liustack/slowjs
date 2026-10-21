// Package layout holds the version of each data file written as a blob
// index (blob.BuildIndex) or as a table of data locale pairs, which the
// generator writing the file writes first and the reader requires, so that
// a file laid out by an older generator is refused rather than misread.
// Change a file's version with any change to what its records hold.
//
// A data set with a package of its own (numdata, datedata, colldata, ...)
// keeps its version there.
package layout

const (
	// Aliases is data/aliases.bin, aliasgen's locale aliases and extension
	// types.
	Aliases byte = 1
	// Available is data/available.bin, availgen's available locales.
	Available byte = 1
	// ICUTree is each data/icutree-<tree>.bin, availgen's index of an ICU
	// tree's bundles, aliases and parents.
	ICUTree byte = 1
	// ICUFallback is data/icufallback.bin, availgen's default scripts and
	// ICU parents.
	ICUFallback byte = 1
	// TimeData is data/timedata.bin, dategen's hour-cycle preferences.
	TimeData byte = 1
	// BreakSets is data/brkitr/sets.bin, segmentgen's break engines' sets
	// and Script property.
	BreakSets byte = 1
	// Properties is data/properties.bin, propgen's Unicode properties.
	Properties byte = 1
	// Pairs is a table of data locale pairs: data/likelysubtags.bin and
	// data/parentlocales.bin, which localegen writes, and each data set's
	// same.bin, which datawrite writes.
	Pairs byte = 1
)
