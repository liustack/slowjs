package temporal

import (
	"errors"
	"fmt"
)

// Temporal throws two kinds of error, and every error this package returns
// wraps one of them, which errors.Is tells apart: ErrRange where a value is
// out of range, and ErrType where one is missing or of the wrong kind.
var (
	ErrRange = errors.New("RangeError")
	ErrType  = errors.New("TypeError")
	// ErrInternal is temporal_rs's assertion and generic errors, which V8
	// throws as a plain Error; Temporal's algorithms do not reach them.
	ErrInternal = errors.New("Error")
)

func rangeError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrRange}, args...)...)
}

func typeError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrType}, args...)...)
}

func internalError(msg string) error {
	return fmt.Errorf("%w: %s", ErrInternal, msg)
}

// assertError is TemporalError::assert: a broken invariant, which carries
// no message in a release build of temporal_rs, as Node's is.
func assertError() error { return internalError("") }
