package avatar

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"avatar-service/internal/domain"
)

const maxFileSize = 10 * 1024 * 1024 // 10 MB

var supportedMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// validateUpload проверяет файл перед загрузкой.
func validateUpload(file multipart.File, header *multipart.FileHeader) (string, error) {
	if header.Size > maxFileSize {
		return "", domain.ErrFileTooLarge
	}
	if header.Size == 0 {
		return "", domain.ErrInvalidInput
	}

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read file header: %w", err)
	}

	contentType := detectMIME(buffer[:n])
	if !supportedMIME[contentType] {
		return "", domain.ErrInvalidFormat
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek file: %w", err)
	}

	return contentType, nil
}

// detectMIME определяет MIME-тип по magic bytes.
func detectMIME(data []byte) string {
	// WebP: RIFF????WEBP
	if len(data) >= 12 &&
		bytes.Equal(data[0:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp"
	}

	return http.DetectContentType(data)
}
