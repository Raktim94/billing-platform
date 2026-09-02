package domain

import "errors"

var (
	ErrNotFound      = errors.New("pricing: not found")
	ErrNegativePrice = errors.New("pricing: price must not be negative")
)
