package main

import (
	"fmt"
	"os"

	"avatar-service/internal/app"
	"avatar-service/internal/runner"
)

// main — точка входа: запускает worker через общий runner.
func main() {
	if err := runner.Run("worker", app.NewWorker); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
