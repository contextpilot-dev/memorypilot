package main

import (
	"os"

	"github.com/contextpilot-dev/memorypilot/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
