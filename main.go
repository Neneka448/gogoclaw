package main

import (
	"os"

	"github.com/Neneka448/gogoclaw/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
