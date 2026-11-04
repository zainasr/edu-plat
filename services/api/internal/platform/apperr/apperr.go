// Package apperr is the one error type that crosses from services to HTTP.
// Services return *Error; httpx maps it to the REQ-API-011 envelope and status.
// Anything else is an internal error and never shows its message to clients.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Field names one invalid input (REQ-API-011 details).
type Field struct {
	Field   string
	Message string
}

// Error is a client-facing failure with a stable code.
type Error struct {
	Status  int
	Code    string
	Message string
	Details []Field
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Validation is a 400 for malformed input or a rule the input breaks.
func Validation(code, message string, details ...Field) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: message, Details: details}
}

// NotFound is a 404. Use it for resources outside the caller's tenant too (API conventions §3).
func NotFound(code, message string) *Error {
	return &Error{Status: http.StatusNotFound, Code: code, Message: message}
}

// Unauthenticated is a 401 for missing, unknown, expired or revoked tokens.
func Unauthenticated(code, message string) *Error {
	return &Error{Status: http.StatusUnauthorized, Code: code, Message: message}
}

// Forbidden is a 403 when the token lacks a named permission.
func Forbidden(code, message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: code, Message: message}
}

// Locked is a 423, used for OTP lockout (REQ-IAM-010).
func Locked(code, message string) *Error {
	return &Error{Status: http.StatusLocked, Code: code, Message: message}
}

// As reports whether err is or wraps an *Error, and returns it.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
