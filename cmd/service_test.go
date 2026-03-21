package cmd

import (
	"strings"
	"testing"
)

func TestGatewayServiceLabelStable(t *testing.T) {
	first := gatewayServiceLabel("/Users/test/.gogoclaw/config.json")
	second := gatewayServiceLabel("/Users/test/.gogoclaw/config.json")
	other := gatewayServiceLabel("/Users/test/.gogoclaw/other.json")

	if first != second {
		t.Fatalf("gatewayServiceLabel should be stable: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("gatewayServiceLabel should differ for different config paths: %q", first)
	}
	if !strings.HasPrefix(first, gatewayServiceLabelPrefix) {
		t.Fatalf("gatewayServiceLabel prefix = %q, want prefix %q", first, gatewayServiceLabelPrefix)
	}
}

func TestRenderGatewayLaunchdPlist(t *testing.T) {
	spec := gatewayServiceSpec{
		Label:       "com.gogoclaw.gateway.deadbeef",
		ProgramArgs: []string{"/Applications/Gogo & Claw.app/Contents/MacOS/gogoclaw", "--config", "/Users/test/.gogoclaw/config.json", "gateway"},
		WorkingDir:  "/Applications/Gogo & Claw.app/Contents/MacOS",
		StdoutPath:  "/Users/test/.gogoclaw/logs/gateway.stdout.log",
		StderrPath:  "/Users/test/.gogoclaw/logs/gateway.stderr.log",
	}

	content, err := renderGatewayLaunchdPlist(spec)
	if err != nil {
		t.Fatalf("renderGatewayLaunchdPlist error = %v", err)
	}

	plist := string(content)
	for _, want := range []string{
		"<key>Label</key>",
		"<string>com.gogoclaw.gateway.deadbeef</string>",
		"<key>ProgramArguments</key>",
		"<string>/Applications/Gogo &amp; Claw.app/Contents/MacOS/gogoclaw</string>",
		"<string>--config</string>",
		"<string>/Users/test/.gogoclaw/config.json</string>",
		"<string>gateway</string>",
		"<key>KeepAlive</key>",
		"<true></true>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q\n%s", want, plist)
		}
	}
}

func TestLaunchctlField(t *testing.T) {
	output := `
com.gogoclaw.gateway.deadbeef = {
	state = running
	pid = 4242
}`

	if got := launchctlField(output, "state"); got != "running" {
		t.Fatalf("launchctlField(state) = %q, want running", got)
	}
	if got := launchctlField(output, "pid"); got != "4242" {
		t.Fatalf("launchctlField(pid) = %q, want 4242", got)
	}
}
