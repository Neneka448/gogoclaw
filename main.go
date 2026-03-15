package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/Neneka448/gogoclaw/cmd"
)

var stderrWriter io.Writer = os.Stderr

func main() {
	os.Exit(runMain(cmd.Execute))
}

func runMain(execute func() error) (exitCode int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			_, _ = fmt.Fprintf(stderrWriter, "Panic: %v\n%s", recovered, debug.Stack())
			exitCode = 1
		}
	}()

	if err := execute(); err != nil {
		_, _ = fmt.Fprintf(stderrWriter, "Error: %v\n", err)
		return 1
	}

	return 0
}
