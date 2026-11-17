package avatar

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net/http"

	"avatar-service/internal/domain"

	// Регистрируем форматы для image.DecodeConfig.
	_ "image/jpeg"
	_ "image/png"
	_ "golang.org/x/image/webp"
)

var supportedMIME = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func validateUpload(file multipart.File, header *multipart.FileHeader) (string, error) {
	if header.Size > domain.MaxFileSize {
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

func detectMIME(data []byte) string {
	if len(data) >= 12 &&
		bytes.Equal(data[0:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	return http.DetectContentType(data)
}

// decodeDimensions определяет ширину и высоту изображения через image.DecodeConfig.
// После вызова позиция чтения сбрасывается в 0 (seek to start).
func decodeDimensions(file multipart.File) (int, int, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("seek before decode: %w", err)
	}
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("decode image config: %w", domain.ErrInvalidFormat)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("seek after decode: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}
