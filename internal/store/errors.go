package store

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	NotFound      ErrorCode = "not-found"      // Store must return this when a record is not found
	AlreadyExists ErrorCode = "already-exists" // Store must return this when a record already exists
	DBConflict    ErrorCode = "db-conflict"    // Store must return this when a DB Txn Conflict occurs (caller must retry Txn)
	DBProblem     ErrorCode = "db-problem"     // Store must return this when the DB returns unexpected errors
)

type ErrorInfo struct {
	Code    ErrorCode // machine-readble ErrorCode enumeration
	Message string    // human-readable debug message (in production, logged on the server only)
	Wrapped error
}

func (e *ErrorInfo) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Wrapped)
	} else {
		return e.Message
	}
}

func (e *ErrorInfo) Is(target error) bool {
	if err, ok := target.(*ErrorInfo); ok {
		return e.Code == err.Code
	}
	return false
}

func (e *ErrorInfo) Unwrap() error {
	return e.Wrapped
}

var NotFoundError = NewErr(NotFound, "not-found")
var AlreadyExistsError = NewErr(AlreadyExists, "already-exists")
var DBConflictError = NewErr(DBConflict, "db-conflict")
var DBProblemError = NewErr(DBProblem, "db-problem")

func NewErr(code ErrorCode, format string, args ...any) error {
	return &ErrorInfo{Code: code, Message: fmt.Sprintf(format, args...)}
}

func WrapErr(code ErrorCode, msg string, err error) error {
	return &ErrorInfo{Code: code, Message: msg, Wrapped: err}
}

func IsNotFoundError(err error) bool {
	return errors.Is(err, NotFoundError)
}

func IsAlreadyExistsError(err error) bool {
	return errors.Is(err, AlreadyExistsError)
}

func IsDBConflictError(err error) bool {
	return errors.Is(err, DBConflictError)
}

func IsDBProblemError(err error) bool {
	return errors.Is(err, DBProblemError)
}
