package errors

import (
	"errors"
	"fmt"
)

// Kind identifies a high-level error category.
type Kind string

const (
	// KindConfig represents configuration related failures.
	KindConfig Kind = "config"
	// KindStorage represents persistence or storage failures.
	KindStorage Kind = "storage"
	// KindBootstrap represents bootstrap/lifecycle failures.
	KindBootstrap Kind = "bootstrap"
	// KindTransport covers transport layer failures.
	KindTransport Kind = "transport"
	// KindVision represents vision pipeline failures.
	KindVision Kind = "vision"
	// KindUnknown is used when no explicit category is available.
	KindUnknown Kind = "unknown"
)

// Error provides a lightweight typed error wrapper.
type Error struct {
	Kind Kind
	Op   string
	Err  error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Op == "" {
		return fmt.Sprintf("%s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Kind, e.Op, e.Err)
}

// Unwrap exposes the nested error for errors.Is/As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Wrap attaches kind/op metadata to an existing error.
func Wrap(kind Kind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}

// IsKind checks whether any error in the chain matches the provided kind.
func IsKind(err error, kind Kind) bool {
	var target *Error
	for err != nil {
		if errors.As(err, &target) {
			return target.Kind == kind
		}
		err = errors.Unwrap(err)
	}
	return false
}
