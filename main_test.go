package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunMainPrintsError(t *testing.T) {
	buffer := &bytes.Buffer{}
	original := stderrWriter
	stderrWriter = buffer
	defer func() {
		stderrWriter = original
	}()

	exitCode := runMain(func() error {
		return errors.New("boom")
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if got := buffer.String(); got != "Error: boom\n" {
		t.Fatalf("stderr = %q, want %q", got, "Error: boom\\n")
	}
}

func TestRunMainPrintsPanic(t *testing.T) {
	buffer := &bytes.Buffer{}
	original := stderrWriter
	stderrWriter = buffer
	defer func() {
		stderrWriter = original
	}()

	exitCode := runMain(func() error {
		panic("kaboom")
	})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if got := buffer.String(); !strings.Contains(got, "Panic: kaboom") {
		t.Fatalf("stderr = %q, want panic header", got)
	}
}