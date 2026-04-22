package main

import (
	"os"

	"github.com/9d4/groupfdn/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
