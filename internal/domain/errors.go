package domain

import (
	"errors"
	"fmt"
)

const (
	ExitOK         = 0
	ExitArgs       = 2
	ExitConfig     = 3
	ExitDependency = 4
	ExitInput      = 5
	ExitDecode     = 6
	ExitNetwork    = 7
	ExitAuth       = 8
	ExitRateLimit  = 9
	ExitProvider   = 10
	ExitWrite      = 11
	ExitPartial    = 12
	ExitCancelled  = 130
)

type AppError struct {
	Code     string
	Message  string
	ExitCode int
	Cause    error
}

func (e *AppError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func NewError(code, message string, exitCode int, cause error) *AppError {
	return &AppError{Code: code, Message: message, ExitCode: exitCode, Cause: cause}
}

func ExitCode(err error) int {
	var appErr *AppError
	if err == nil {
		return ExitOK
	}
	if errors.As(err, &appErr) {
		return appErr.ExitCode
	}
	return ExitProvider
}

func ErrorCode(err error) string {
	var appErr *AppError
	if err == nil {
		return ""
	}
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return "unknown_error"
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
