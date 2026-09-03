package domain

import "errors"

var (
	ErrNotFound           = errors.New("accounting: not found")
	ErrUnbalancedJournal  = errors.New("accounting: journal debits and credits do not sum equal")
	ErrEmptyJournal       = errors.New("accounting: journal must have at least two lines")
	ErrAccountNotFound    = errors.New("accounting: account not found")
	ErrPeriodLocked       = errors.New("accounting: fiscal period is locked")
	ErrChartAlreadyExists = errors.New("accounting: chart of accounts already exists for this organisation")
)
