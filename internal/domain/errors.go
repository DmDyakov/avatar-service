package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")

	ErrInvalidFormat = errors.New("invalid file format")
	ErrFileTooLarge  = errors.New("file too large")

	ErrAlreadyProcessed = errors.New("already processed")
	ErrProcessingFailed = errors.New("processing failed")
	ErrMaxAttempts      = errors.New("max attempts reached")
)
