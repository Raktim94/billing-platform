package domain

import "errors"

var (
	ErrNotFound            = errors.New("inventory: not found")
	ErrInvalidMovementType = errors.New("inventory: invalid movement type")
	ErrInsufficientStock   = errors.New("inventory: insufficient stock")
	ErrDuplicateBatchCode  = errors.New("inventory: batch code already in use for this product")
	ErrDuplicateSerial     = errors.New("inventory: serial number already in use for this product")
)
