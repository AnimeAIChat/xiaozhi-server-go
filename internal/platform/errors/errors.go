package errors

import (
	"errors"
	"fmt"
)

type Kind string

const (
	KindConfig     Kind = "config"
	KindDomain     Kind = "domain"
	KindTransport  Kind = "transport"
	KindPlatform   Kind = "platform"
	KindBootstrap  Kind = "bootstrap"
	KindStorage    Kind = "storage"
	KindVision     Kind = "vision"
	KindUnknown    Kind = "unknown"
)

type Error struct {
	Kind    Kind
	Op      string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s:%s] %s: %v", e.Kind, e.Op, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s:%s] %s", e.Kind, e.Op, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func Wrap(kind Kind, op, message string, err error) *Error {
	if err == nil {
		return nil
	}

	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}

	return &Error{
		Kind:    kind,
		Op:      op,
		Message: message,
		Cause:   err,
	}
}

func New(kind Kind, op, message string) *Error {
	return &Error{
		Kind:    kind,
		Op:      op,
		Message: message,
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
