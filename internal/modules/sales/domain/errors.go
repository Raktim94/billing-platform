package domain

import "errors"

var (
	ErrNotFound             = errors.New("sales: not found")
	ErrInvalidDocumentType  = errors.New("sales: invalid document type")
	ErrDocumentNotDraft     = errors.New("sales: document is not in DRAFT status")
	ErrDocumentNotFinalized = errors.New("sales: reference document is not FINALIZED")
	ErrEmptyDocument        = errors.New("sales: document has no lines")
	ErrDuplicateNumber      = errors.New("sales: document number already in use for this document type")
)
