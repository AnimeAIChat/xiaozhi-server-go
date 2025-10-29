package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		contains []string
	}{
		{
			name: "error with cause",
			err: Wrap(KindConfig, "load", "failed to load config",
				errors.New("file not found")),
			contains: []string{"[config:load]", "failed to load config", "file not found"},
		},
		{
			name: "error without cause",
			err: New(KindDomain, "validate", "invalid input"),
			contains: []string{"[domain:validate]", "invalid input"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			for _, substr := range tt.contains {
				if !strings.Contains(errStr, substr) {
					t.Errorf("error string %q does not contain %q", errStr, substr)
				}
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	wrappedErr := Wrap(KindConfig, "test", "wrapped", originalErr)

	if !errors.Is(wrappedErr, originalErr) {
		t.Error("Unwrap should return the original error")
	}
}

func TestIsKind(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		kind     Kind
		expected bool
	}{
		{
			name:     "direct error kind match",
			err:      New(KindConfig, "test", "message"),
			kind:     KindConfig,
			expected: true,
		},
		{
			name:     "wrapped error kind match",
			err:      Wrap(KindDomain, "test", "message", errors.New("cause")),
			kind:     KindDomain,
			expected: true,
		},
		{
			name:     "error kind mismatch",
			err:      New(KindConfig, "test", "message"),
			kind:     KindDomain,
			expected: false,
		},
		{
			name:     "non-typed error",
			err:      errors.New("plain error"),
			kind:     KindConfig,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsKind(tt.err, tt.kind)
			if result != tt.expected {
				t.Errorf("IsKind() = %v, expected %v", result, tt.expected)
			}
		})
	}
}