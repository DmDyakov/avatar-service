package avatar

import (
	"bytes"
	"mime/multipart"
	"testing"

	"avatar-service/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUpload(t *testing.T) {
	t.Run("valid jpeg", func(t *testing.T) {
		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes())

		contentType, err := validateUpload(file, header)
		require.NoError(t, err)
		assert.Equal(t, "image/jpeg", contentType)
	})

	t.Run("valid png", func(t *testing.T) {
		file, header := createMultipartFile(t, "avatar.png", pngBytes())

		contentType, err := validateUpload(file, header)
		require.NoError(t, err)
		assert.Equal(t, "image/png", contentType)
	})

	t.Run("valid webp", func(t *testing.T) {
		file, header := createMultipartFile(t, "avatar.webp", webpBytes())

		contentType, err := validateUpload(file, header)
		require.NoError(t, err)
		assert.Equal(t, "image/webp", contentType)
	})

	t.Run("file too large", func(t *testing.T) {
		header := &multipart.FileHeader{
			Filename: "big.jpg",
			Size:     maxFileSize + 1,
		}

		_, err := validateUpload(nil, header)
		assert.ErrorIs(t, err, domain.ErrFileTooLarge)
	})

	t.Run("empty file", func(t *testing.T) {
		header := &multipart.FileHeader{
			Filename: "empty.jpg",
			Size:     0,
		}

		_, err := validateUpload(nil, header)
		assert.ErrorIs(t, err, domain.ErrInvalidInput)
	})

	t.Run("invalid format - text file", func(t *testing.T) {
		file, header := createMultipartFile(t, "test.txt", []byte("hello world"))

		_, err := validateUpload(file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidFormat)
	})

	t.Run("invalid format - pdf", func(t *testing.T) {
		// PDF magic bytes: %PDF
		pdfBytes := []byte("%PDF-1.4\n...")
		file, header := createMultipartFile(t, "doc.pdf", pdfBytes)

		_, err := validateUpload(file, header)
		assert.ErrorIs(t, err, domain.ErrInvalidFormat)
	})

	t.Run("resets reader position", func(t *testing.T) {
		file, header := createMultipartFile(t, "avatar.jpg", jpegBytes())

		_, err := validateUpload(file, header)
		require.NoError(t, err)

		// После валидации reader должен быть в начале
		buf := make([]byte, 3)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, []byte{0xFF, 0xD8, 0xFF}, buf) // JPEG magic bytes
	})
}

// createMultipartFile создаёт multipart.File и FileHeader для тестов.
func createMultipartFile(t *testing.T, filename string, content []byte) (multipart.File, *multipart.FileHeader) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	_, err = part.Write(content)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	reader := multipart.NewReader(body, writer.Boundary())
	form, err := reader.ReadForm(int64(len(content)) + 1024)
	require.NoError(t, err)


	fileHeader := form.File["file"][0]

	file, err := fileHeader.Open()
	require.NoError(t, err)

	t.Cleanup(func() {
		file.Close()
		form.RemoveAll()
	})

	return file, fileHeader
}

// jpegBytes возвращает минимальный валидный JPEG (magic bytes).
func jpegBytes() []byte {
	// JPEG: FF D8 FF
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 100)...)
}

// pngBytes возвращает минимальный валидный PNG (magic bytes).
func pngBytes() []byte {
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	return append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 100)...)
}

// webpBytes возвращает минимальный валидный WebP (magic bytes).
func webpBytes() []byte {
	// WebP: RIFF....WEBP
	header := []byte{
		0x52, 0x49, 0x46, 0x46, // "RIFF"
		0x00, 0x00, 0x00, 0x00, // size
		0x57, 0x45, 0x42, 0x50, // "WEBP"
	}
	return append(header, bytes.Repeat([]byte{0x00}, 100)...)
}
