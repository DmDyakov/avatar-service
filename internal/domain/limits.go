package domain

// MaxFileSize — максимальный размер загружаемого файла (10 MB).
// Используется при валидации в сервисе и в 413-ответе handler'а.
const MaxFileSize int64 = 10 << 20
