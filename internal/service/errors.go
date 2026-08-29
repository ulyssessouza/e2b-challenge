package service

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrQuotaExceeded = errors.New("quota exceeded")
)
