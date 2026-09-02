package domain

import "errors"

var (
	ErrNotFound      = errors.New("catalogue: not found")
	ErrDuplicateSKU  = errors.New("catalogue: sku code already in use")
	ErrDuplicateCode = errors.New("catalogue: unit code already in use")
)
