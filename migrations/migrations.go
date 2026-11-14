// Package migrations содержит встроенные SQL миграции.
package migrations

import "embed"

// FS содержит SQL миграции.
//
//go:embed *.sql
var FS embed.FS
