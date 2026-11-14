// Package buildinfo предоставляет информацию о версии сборки приложения.
//
// Пакет содержит глобальные переменные, которые заполняются при сборке
// с помощью флагов линковщика -ldflags. Если переменные не заполнены,
// при выводе используется значение "N/A".
//
// Использование при сборке:
//
//	go build -ldflags "-X your-module/pkg/buildinfo.Version=v1.0.0 \
//	                   -X your-module/pkg/buildinfo.Date=$(date -u +%Y-%m-%d) \
//	                   -X your-module/pkg/buildinfo.Commit=$(git rev-parse HEAD)" \
//	                   ./cmd/server/
//
// Использование в коде:
//
//	import "your-module/pkg/buildinfo"
//
//	func main() {
//	    buildinfo.Print()
//	}
package buildinfo

import "fmt"

var (
	Version string
	Date    string
	Commit  string
)

// Print выводит информацию о версии сборки в stdout.
func Print() {
	fmt.Printf("Build version: %s\n", valueOrDefault(Version))
	fmt.Printf("Build date: %s\n", valueOrDefault(Date))
	fmt.Printf("Build commit: %s\n", valueOrDefault(Commit))
}

func valueOrDefault(v string) string {
	if v == "" {
		return "N/A"
	}
	return v
}
