package context

import (
	"testing"
)

func TestValidateInvocationModeAcceptsValidModes(t *testing.T) {
	for _, mode := range []string{"foreground", "background", "cron", ""} {
		if err := ValidateInvocationMode(mode); err != nil {
			t.Errorf("ValidateInvocationMode(%q) = %v, want nil", mode, err)
		}
	}
}

func TestValidateInvocationModeRejectsInvalidMode(t *testing.T) {
	for _, mode := range []string{"invalid", "FOREGROUND", "bg", "scheduled"} {
		if err := ValidateInvocationMode(mode); err == nil {
			t.Errorf("ValidateInvocationMode(%q) = nil, want error", mode)
		}
	}
}
