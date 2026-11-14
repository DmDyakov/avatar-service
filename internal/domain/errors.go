package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")

	ErrInvalidFormat = errors.New("invalid file format")
	ErrFileTooLarge  = errors.New("file too large")

	// ErrStorageUnavailable — запись в БД есть, но файл не удалось получить
	ErrStorageUnavailable = errors.New("storage unavailable")
)
