package main

import (
	"fmt"
	"os"

	"avatar-service/internal/app"
	"avatar-service/internal/runner"
)

func main() {
	if err := runner.Run("relay", app.NewRelay); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
