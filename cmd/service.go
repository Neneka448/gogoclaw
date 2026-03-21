package cmd

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const gatewayServiceLabelPrefix = "com.gogoclaw.gateway."

type gatewayServiceSpec struct {
	Label         string
	ConfigPath    string
	ProgramPath   string
	PlistPath     string
	StdoutPath    string
	StderrPath    string
	WorkingDir    string
	DomainTarget  string
	ServiceTarget string
	ProgramArgs   []string
}

type launchdPlist struct {
	XMLName xml.Name    `xml:"plist"`
	Version string      `xml:"version,attr"`
	Dict    launchdDict `xml:"dict"`
}

type launchdDict struct {
	Pairs []launchdValue `xml:",any"`
}

type launchdValue struct {
	XMLName xml.Name
	Value   string         `xml:",chardata"`
	Items   []launchdValue `xml:",any,omitempty"`
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage the macOS launchd service for gogoclaw gateway",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGatewayServiceStatus(cmd)
	},
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the launchd service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := resolveGatewayServiceSpec(cfgFile)
		if err != nil {
			return err
		}
		if err := installGatewayService(spec); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installed launchd service %s\n", spec.Label)
		fmt.Fprintf(cmd.OutOrStdout(), "Plist:  %s\n", spec.PlistPath)
		fmt.Fprintf(cmd.OutOrStdout(), "Stdout: %s\n", spec.StdoutPath)
		fmt.Fprintf(cmd.OutOrStdout(), "Stderr: %s\n", spec.StderrPath)
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and uninstall the launchd service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := resolveGatewayServiceSpec(cfgFile)
		if err != nil {
			return err
		}
		if err := uninstallGatewayService(spec); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed launchd service %s\n", spec.Label)
		return nil
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the installed launchd service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := resolveGatewayServiceSpec(cfgFile)
		if err != nil {
			return err
		}
		if err := startGatewayService(spec); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Started launchd service %s\n", spec.Label)
		return nil
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the launchd service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := resolveGatewayServiceSpec(cfgFile)
		if err != nil {
			return err
		}
		if err := stopGatewayService(spec); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped launchd service %s\n", spec.Label)
		return nil
	},
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the launchd service",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := resolveGatewayServiceSpec(cfgFile)
		if err != nil {
			return err
		}
		if err := restartGatewayService(spec); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Restarted launchd service %s\n", spec.Label)
		return nil
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show launchd service status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGatewayServiceStatus(cmd)
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
}

func runGatewayServiceStatus(cmd *cobra.Command) error {
	spec, err := resolveGatewayServiceSpec(cfgFile)
	if err != nil {
		return err
	}

	installed := fileExists(spec.PlistPath)
	loaded, output, err := gatewayServiceLoaded(spec)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Label:      %s\n", spec.Label)
	fmt.Fprintf(cmd.OutOrStdout(), "Installed:  %t\n", installed)
	fmt.Fprintf(cmd.OutOrStdout(), "Loaded:     %t\n", loaded)
	fmt.Fprintf(cmd.OutOrStdout(), "Plist:      %s\n", spec.PlistPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Config:     %s\n", spec.ConfigPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Stdout log: %s\n", spec.StdoutPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Stderr log: %s\n", spec.StderrPath)

	if loaded {
		state := launchctlField(output, "state")
		pid := launchctlField(output, "pid")
		if state != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "State:      %s\n", state)
		}
		if pid != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "PID:        %s\n", pid)
		}
	}

	return nil
}

func installGatewayService(spec gatewayServiceSpec) error {
	if err := os.MkdirAll(filepath.Dir(spec.PlistPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(spec.StdoutPath), 0755); err != nil {
		return err
	}

	content, err := renderGatewayLaunchdPlist(spec)
	if err != nil {
		return err
	}
	if err := os.WriteFile(spec.PlistPath, content, 0644); err != nil {
		return err
	}

	loaded, _, err := gatewayServiceLoaded(spec)
	if err != nil {
		return err
	}
	if loaded {
		if err := stopGatewayService(spec); err != nil {
			return err
		}
	}
	return startGatewayService(spec)
}

func uninstallGatewayService(spec gatewayServiceSpec) error {
	loaded, _, err := gatewayServiceLoaded(spec)
	if err != nil {
		return err
	}
	if loaded {
		if err := stopGatewayService(spec); err != nil {
			return err
		}
	}
	if err := os.Remove(spec.PlistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func startGatewayService(spec gatewayServiceSpec) error {
	if !fileExists(spec.PlistPath) {
		return fmt.Errorf("launchd plist does not exist: %s", spec.PlistPath)
	}

	loaded, _, err := gatewayServiceLoaded(spec)
	if err != nil {
		return err
	}
	if !loaded {
		if err := runLaunchctl(spec.DomainTarget, "bootstrap", spec.DomainTarget, spec.PlistPath); err != nil {
			return err
		}
	}
	if err := runLaunchctl(spec.ServiceTarget, "kickstart", "-k", spec.ServiceTarget); err != nil {
		return err
	}
	return nil
}

func stopGatewayService(spec gatewayServiceSpec) error {
	loaded, _, err := gatewayServiceLoaded(spec)
	if err != nil {
		return err
	}
	if !loaded {
		return nil
	}
	return runLaunchctl(spec.Label, "bootout", spec.DomainTarget, spec.PlistPath)
}

func restartGatewayService(spec gatewayServiceSpec) error {
	if err := stopGatewayService(spec); err != nil {
		return err
	}
	return startGatewayService(spec)
}

func gatewayServiceLoaded(spec gatewayServiceSpec) (bool, string, error) {
	output, err := runLaunchctlOutput("print", spec.ServiceTarget)
	if err != nil {
		return false, output, nil
	}
	return true, output, nil
}

func resolveGatewayServiceSpec(configPath string) (gatewayServiceSpec, error) {
	if runtime.GOOS != "darwin" {
		return gatewayServiceSpec{}, fmt.Errorf("service command is only supported on macOS")
	}

	resolvedConfigPath, err := resolveConfigPath(configPath)
	if err != nil {
		return gatewayServiceSpec{}, err
	}
	resolvedConfigPath, err = normalizeUserPath(resolvedConfigPath)
	if err != nil {
		return gatewayServiceSpec{}, err
	}

	programPath, err := os.Executable()
	if err != nil {
		return gatewayServiceSpec{}, err
	}
	programPath, err = filepath.EvalSymlinks(programPath)
	if err != nil {
		return gatewayServiceSpec{}, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return gatewayServiceSpec{}, err
	}
	uid := os.Getuid()
	label := gatewayServiceLabel(resolvedConfigPath)
	logDir := filepath.Join(filepath.Dir(resolvedConfigPath), "logs")
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist")

	spec := gatewayServiceSpec{
		Label:         label,
		ConfigPath:    resolvedConfigPath,
		ProgramPath:   programPath,
		PlistPath:     plistPath,
		StdoutPath:    filepath.Join(logDir, "gateway.stdout.log"),
		StderrPath:    filepath.Join(logDir, "gateway.stderr.log"),
		WorkingDir:    filepath.Dir(programPath),
		DomainTarget:  fmt.Sprintf("gui/%d", uid),
		ServiceTarget: fmt.Sprintf("gui/%d/%s", uid, label),
		ProgramArgs: []string{
			programPath,
			"--config",
			resolvedConfigPath,
			"gateway",
		},
	}
	return spec, nil
}

func gatewayServiceLabel(configPath string) string {
	sum := sha1.Sum([]byte(filepath.Clean(configPath)))
	return gatewayServiceLabelPrefix + hex.EncodeToString(sum[:6])
}

func renderGatewayLaunchdPlist(spec gatewayServiceSpec) ([]byte, error) {
	arrayItems := make([]launchdValue, 0, len(spec.ProgramArgs))
	for _, arg := range spec.ProgramArgs {
		arrayItems = append(arrayItems, launchdValue{XMLName: xml.Name{Local: "string"}, Value: arg})
	}

	doc := launchdPlist{
		Version: "1.0",
		Dict: launchdDict{
			Pairs: []launchdValue{
				{XMLName: xml.Name{Local: "key"}, Value: "Label"},
				{XMLName: xml.Name{Local: "string"}, Value: spec.Label},
				{XMLName: xml.Name{Local: "key"}, Value: "ProgramArguments"},
				{XMLName: xml.Name{Local: "array"}, Items: arrayItems},
				{XMLName: xml.Name{Local: "key"}, Value: "WorkingDirectory"},
				{XMLName: xml.Name{Local: "string"}, Value: spec.WorkingDir},
				{XMLName: xml.Name{Local: "key"}, Value: "RunAtLoad"},
				{XMLName: xml.Name{Local: "true"}},
				{XMLName: xml.Name{Local: "key"}, Value: "KeepAlive"},
				{XMLName: xml.Name{Local: "true"}},
				{XMLName: xml.Name{Local: "key"}, Value: "StandardOutPath"},
				{XMLName: xml.Name{Local: "string"}, Value: spec.StdoutPath},
				{XMLName: xml.Name{Local: "key"}, Value: "StandardErrorPath"},
				{XMLName: xml.Name{Local: "string"}, Value: spec.StderrPath},
			},
		},
	}

	content, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}

	header := []byte(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	content = append(header, content...)
	content = append(content, '\n')
	return content, nil
}

func normalizeUserPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func runLaunchctl(label string, args ...string) error {
	output, err := runLaunchctlOutput(args...)
	if err != nil {
		message := strings.TrimSpace(output)
		if message == "" {
			return fmt.Errorf("launchctl %s failed: %w", label, err)
		}
		return fmt.Errorf("launchctl %s failed: %w: %s", label, err, message)
	}
	return nil
}

func runLaunchctlOutput(args ...string) (string, error) {
	cmd := exec.Command("launchctl", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func launchctlField(output string, field string) string {
	prefix := field + " = "
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), ";")
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
