package domain

import "errors"

var ErrDuplicate = errors.New("already exists")

var ErrInternal = errors.New("internal server error")

var ErrInvalidSignature = errors.New("invalid signature")

var ErrNotFound = errors.New("not found")

var ErrInvalidID = errors.New("invalid id")

var ErrValidation = errors.New("validation failed")

var ErrConflict = errors.New("conflict")

func IsTransient(err error) bool {
	return errors.Is(err, ErrInternal)
}

type taggedError struct {
	msg    string
	target error
}

func (e *taggedError) Error() string        { return e.msg }
func (e *taggedError) Is(target error) bool { return target == e.target }

func NotFound(msg string) error { return &taggedError{msg: msg, target: ErrNotFound} }

func InvalidID(msg string) error { return &taggedError{msg: msg, target: ErrInvalidID} }

func Duplicate(msg string) error { return &taggedError{msg: msg, target: ErrDuplicate} }

func Validation(msg string) error { return &taggedError{msg: msg, target: ErrValidation} }

func Conflict(msg string) error { return &taggedError{msg: msg, target: ErrConflict} }
