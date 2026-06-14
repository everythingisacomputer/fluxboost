package main

import (
	"os"

	"github.com/everythingisacomputer/fluxboost/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
