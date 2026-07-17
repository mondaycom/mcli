// Package errs defines structured error types and exit-code mapping for mcli.
package errs

import (
	"errors"
	"fmt"
)

// Code is a stable string code identifying the class of error.
type Code string

const (
	// CodeUsage indicates a user input or flag error.
	CodeUsage Code = "USAGE"
	// CodeAuth indicates a missing or invalid API token.
	CodeAuth Code = "AUTH"
	// CodeAPI indicates a Monday API error (non-2xx or GraphQL errors).
	CodeAPI Code = "API"
	// CodeRateLimited indicates a 429 / complexity-budget exhaustion after retries.
	CodeRateLimited Code = "RATE_LIMITED"
	// CodeNotFound indicates a resource was not found.
	CodeNotFound Code = "NOT_FOUND"
	// CodeConflict indicates a resource conflict.
	CodeConflict Code = "CONFLICT"
	// CodeInternal indicates an unexpected internal error.
	CodeInternal Code = "INTERNAL"
	// CodeDaemonRequired indicates the mcli daemon is not running but is needed.
	CodeDaemonRequired Code = "DAEMON_REQUIRED"
	// CodeInterrupted indicates the operation was cancelled (SIGTERM/SIGINT/context cancel).
	CodeInterrupted Code = "INTERRUPTED"
)

// Error is a structured error value carrying a Code, a human message, and an
// optional wrapped cause.
type Error struct {
	Code    Code
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause, implementing errors.Unwrap.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Usage constructs a CodeUsage Error.
func Usage(format string, args ...any) *Error {
	return &Error{Code: CodeUsage, Message: fmt.Sprintf(format, args...)}
}

// Auth constructs a CodeAuth Error.
func Auth(format string, args ...any) *Error {
	return &Error{Code: CodeAuth, Message: fmt.Sprintf(format, args...)}
}

// API constructs a CodeAPI Error.
func API(format string, args ...any) *Error {
	return &Error{Code: CodeAPI, Message: fmt.Sprintf(format, args...)}
}

// RateLimited constructs a CodeRateLimited Error.
func RateLimited(format string, args ...any) *Error {
	return &Error{Code: CodeRateLimited, Message: fmt.Sprintf(format, args...)}
}

// NotFound constructs a CodeNotFound Error.
func NotFound(format string, args ...any) *Error {
	return &Error{Code: CodeNotFound, Message: fmt.Sprintf(format, args...)}
}

// Internal constructs a CodeInternal Error.
func Internal(format string, args ...any) *Error {
	return &Error{Code: CodeInternal, Message: fmt.Sprintf(format, args...)}
}

// DaemonRequired constructs a CodeDaemonRequired Error.
func DaemonRequired(format string, args ...any) *Error {
	return &Error{Code: CodeDaemonRequired, Message: fmt.Sprintf(format, args...)}
}

// Interrupted constructs a CodeInterrupted Error.
func Interrupted(format string, args ...any) *Error {
	return &Error{Code: CodeInterrupted, Message: fmt.Sprintf(format, args...)}
}

// ToExitCode maps an error to an exit code per ADR-002.
// Returns 0 if err is nil.
func ToExitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := errors.AsType[*Error](err); ok {
		switch e.Code {
		case CodeUsage:
			return 1
		case CodeAPI, CodeNotFound:
			return 2
		case CodeAuth:
			return 3
		case CodeRateLimited:
			return 4
		case CodeInternal:
			return 5
		case CodeDaemonRequired:
			return 6
		case CodeInterrupted:
			return 130
		}
	}
	return 5
}
