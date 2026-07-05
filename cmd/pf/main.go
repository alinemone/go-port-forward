package main

import (
	"os"

	"github.com/alinemone/go-port-forward/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
