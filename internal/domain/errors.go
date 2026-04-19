package domain

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrorCodeNotFound      ErrorCode = "NOT_FOUND"
	ErrorCodeValidation    ErrorCode = "VALIDATION_ERROR"
	ErrorCodeConflict      ErrorCode = "CONFLICT"
	ErrorCodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	ErrorCodeInternal      ErrorCode = "INTERNAL_ERROR"
	defaultInternalMessage           = "Unexpected server error"
)

type AppError struct {
	Code    ErrorCode
	Message string
	Details map[string]any
	Err     error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}

	if e.Message != "" {
		return e.Message
	}

	if e.Err != nil {
		return e.Err.Error()
	}

	return string(e.Code)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

func (e *AppError) WithCause(err error) *AppError {
	if e == nil {
		return nil
	}
	e.Err = err
	return e
}

func NotFound(message string, details map[string]any) *AppError {
	return &AppError{Code: ErrorCodeNotFound, Message: message, Details: details}
}

func Validation(message string, details map[string]any) *AppError {
	return &AppError{Code: ErrorCodeValidation, Message: message, Details: details}
}

func Conflict(message string, details map[string]any) *AppError {
	return &AppError{Code: ErrorCodeConflict, Message: message, Details: details}
}

func Unauthorized(message string, details map[string]any) *AppError {
	return &AppError{Code: ErrorCodeUnauthorized, Message: message, Details: details}
}

func Internal(message string, details map[string]any) *AppError {
	if message == "" {
		message = defaultInternalMessage
	}

	return &AppError{Code: ErrorCodeInternal, Message: message, Details: details}
}

func AsAppError(err error) *AppError {
	if err == nil {
		return nil
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	return Internal(defaultInternalMessage, nil).WithCause(err)
}

func Errorf(code ErrorCode, message string, details map[string]any, format string, args ...any) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Details: details,
		Err:     fmt.Errorf(format, args...),
	}
}
