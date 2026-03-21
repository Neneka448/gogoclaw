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
const codesignDefaultIdentifier = "com.gogoclaw.gogoclaw"
const codesignCertCN = "GoGoClaw Code Signing"

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

var serviceSignCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign the gogoclaw binary with a stable macOS code identity",
	Long: `Sign the gogoclaw binary with a stable code signing identity.

By default, uses ad-hoc signing (signer "-") with a stable identifier
"com.gogoclaw.gogoclaw". This gives the binary a persistent identity
that macOS NECP and Network Extension policies can recognize.

Use --signer to specify a signing certificate name from your Keychain,
for example a self-signed certificate or an Apple Developer ID.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		signer, _ := cmd.Flags().GetString("signer")
		identifier, _ := cmd.Flags().GetString("identifier")
		return runServiceSign(cmd, signer, identifier)
	},
}

var serviceSetupCertCmd = &cobra.Command{
	Use:   "setup-cert",
	Short: "Create a self-signed code signing certificate in Keychain",
	Long: `Create a self-signed code signing certificate and install it into
the login keychain. This gives the gogoclaw binary a real signing
authority and TeamIdentifier that macOS NECP can recognize, which
improves compatibility with VPN/proxy apps like Clash.

After running this command, use "service sign --signer ` + `"` + codesignCertCN + `"` + `"
to sign the binary with the certificate.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetupCert(cmd)
	},
}

var serviceBundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Create a minimal macOS .app bundle around the gogoclaw binary",
	Long: `Create a GoGoClaw.app bundle in the specified directory (default:
~/Applications/). The bundle gives macOS a full application identity
with CFBundleIdentifier, which is the strongest way to establish a
stable NECP identity for background network access.

After bundling, use "service install" to reinstall the launchd service
pointing to the bundled binary.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		installDir, _ := cmd.Flags().GetString("dir")
		signer, _ := cmd.Flags().GetString("signer")
		return runServiceBundle(cmd, installDir, signer)
	},
}

var serviceGrantNetworkCmd = &cobra.Command{
	Use:   "grant-network",
	Short: "Trigger the macOS Local Network permission prompt",
	Long: `On macOS 15+, apps need explicit user consent for local network access.
This command opens the GoGoClaw.app bundle to trigger the system
permission dialog. Accept the prompt to allow local network access,
then restart the launchd service.

This must be run from a GUI session (e.g., Terminal.app on the Mac,
or screen sharing).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGrantNetwork(cmd)
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
	serviceCmd.AddCommand(serviceSignCmd)
	serviceCmd.AddCommand(serviceSetupCertCmd)
	serviceCmd.AddCommand(serviceBundleCmd)
	serviceCmd.AddCommand(serviceGrantNetworkCmd)

	serviceSignCmd.Flags().String("signer", "-", "Code signing identity (use \"-\" for ad-hoc, or a Keychain certificate name)")
	serviceSignCmd.Flags().String("identifier", codesignDefaultIdentifier, "Bundle identifier for codesign")

	homeDir, _ := os.UserHomeDir()
	defaultBundleDir := filepath.Join(homeDir, "Applications")
	serviceBundleCmd.Flags().String("dir", defaultBundleDir, "Directory to create the .app bundle in")
	serviceBundleCmd.Flags().String("signer", "-", "Code signing identity for the bundle")
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

	sigInfo := codesignInfo(spec.ProgramPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Binary:     %s\n", spec.ProgramPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Signed:     %s\n", sigInfo)

	return nil
}

func runServiceSign(cmd *cobra.Command, signer string, identifier string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("code signing is only supported on macOS")
	}

	programPath, err := os.Executable()
	if err != nil {
		return err
	}
	programPath, err = filepath.EvalSymlinks(programPath)
	if err != nil {
		return err
	}

	args := []string{
		"--force",
		"--sign", signer,
		"--identifier", identifier,
		"--options", "runtime",
		programPath,
	}
	out, err := exec.Command("codesign", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Signed %s\n", programPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  Identifier: %s\n", identifier)
	fmt.Fprintf(cmd.OutOrStdout(), "  Signer:     %s\n", signerDisplay(signer))

	info := codesignInfo(programPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  Verify:     %s\n", info)
	return nil
}

func codesignInfo(path string) string {
	out, err := exec.Command("codesign", "-dvv", path).CombinedOutput()
	if err != nil {
		return "unsigned"
	}
	output := string(out)
	id := extractCodesignField(output, "Identifier")
	authority := extractCodesignField(output, "Authority")
	flags := extractCodesignField(output, "CodeDirectory flags")

	var parts []string
	if id != "" {
		parts = append(parts, "id="+id)
	}
	if authority != "" {
		parts = append(parts, "authority="+authority)
	} else {
		parts = append(parts, "authority=ad-hoc")
	}
	if strings.Contains(flags, "runtime") {
		parts = append(parts, "runtime=hardened")
	}
	if len(parts) == 0 {
		return "signed (details unavailable)"
	}
	return strings.Join(parts, ", ")
}

func extractCodesignField(output string, field string) string {
	prefix := field + "="
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func signerDisplay(signer string) string {
	if signer == "-" {
		return "ad-hoc (no certificate)"
	}
	return signer
}

func runSetupCert(cmd *cobra.Command) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("certificate setup is only supported on macOS")
	}

	// Check if certificate already exists
	out, err := exec.Command("security", "find-identity", "-v", "-p", "codesigning").CombinedOutput()
	if err == nil && strings.Contains(string(out), codesignCertCN) {
		fmt.Fprintf(cmd.OutOrStdout(), "Certificate %q already exists in Keychain.\n", codesignCertCN)
		fmt.Fprintf(cmd.OutOrStdout(), "To sign: gogoclaw service sign --signer %q\n", codesignCertCN)
		return nil
	}

	// Create a temporary directory for cert generation
	tmpDir, err := os.MkdirTemp("", "gogoclaw-cert-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "key.pem")
	certPath := filepath.Join(tmpDir, "cert.pem")
	p12Path := filepath.Join(tmpDir, "cert.p12")

	// Generate self-signed code signing certificate
	opensslConf := filepath.Join(tmpDir, "openssl.cnf")
	confContent := `[req]
distinguished_name = req_dn
x509_extensions = codesign
prompt = no

[req_dn]
CN = ` + codesignCertCN + `
O = GoGoClaw

[codesign]
keyUsage = critical, digitalSignature
extendedKeyUsage = codeSigning
basicConstraints = critical, CA:false
`
	if err := os.WriteFile(opensslConf, []byte(confContent), 0600); err != nil {
		return fmt.Errorf("write openssl config: %w", err)
	}

	// Generate key + cert
	genOut, err := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048",
		"-keyout", keyPath, "-out", certPath,
		"-days", "3650", "-nodes",
		"-config", opensslConf).CombinedOutput()
	if err != nil {
		return fmt.Errorf("generate certificate: %w: %s", err, strings.TrimSpace(string(genOut)))
	}

	// Convert to PKCS12 (use a simple passphrase for macOS Security framework compatibility)
	const p12Pass = "gogoclaw"
	p12Out, err := exec.Command("openssl", "pkcs12", "-export",
		"-out", p12Path, "-inkey", keyPath, "-in", certPath,
		"-passout", "pass:"+p12Pass).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create p12: %w: %s", err, strings.TrimSpace(string(p12Out)))
	}

	// Import into login keychain
	importOut, err := exec.Command("security", "import", p12Path,
		"-k", filepath.Join(os.Getenv("HOME"), "Library", "Keychains", "login.keychain-db"),
		"-P", p12Pass,
		"-T", "/usr/bin/codesign").CombinedOutput()
	if err != nil {
		return fmt.Errorf("import to keychain: %w: %s", err, strings.TrimSpace(string(importOut)))
	}

	// Trust the certificate for code signing
	trustOut, err := exec.Command("security", "add-trusted-cert",
		"-p", "codeSign",
		"-k", filepath.Join(os.Getenv("HOME"), "Library", "Keychains", "login.keychain-db"),
		certPath).CombinedOutput()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Warning: could not auto-trust certificate: %s\n", strings.TrimSpace(string(trustOut)))
		fmt.Fprintf(cmd.OutOrStdout(), "You may need to trust it manually in Keychain Access.\n")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created code signing certificate %q in login keychain.\n", codesignCertCN)
	fmt.Fprintf(cmd.OutOrStdout(), "To sign the binary:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  gogoclaw service sign --signer %q\n", codesignCertCN)
	fmt.Fprintf(cmd.OutOrStdout(), "To build + bundle:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  gogoclaw service bundle --signer %q\n", codesignCertCN)
	return nil
}

func runServiceBundle(cmd *cobra.Command, installDir string, signer string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("app bundling is only supported on macOS")
	}

	programPath, err := os.Executable()
	if err != nil {
		return err
	}
	programPath, err = filepath.EvalSymlinks(programPath)
	if err != nil {
		return err
	}

	appDir := filepath.Join(installDir, "GoGoClaw.app")
	macosDir := filepath.Join(appDir, "Contents", "MacOS")
	binaryDst := filepath.Join(macosDir, "gogoclaw")
	infoPlistPath := filepath.Join(appDir, "Contents", "Info.plist")

	if err := os.MkdirAll(macosDir, 0755); err != nil {
		return fmt.Errorf("create app bundle directory: %w", err)
	}

	// Copy binary into the bundle
	srcData, err := os.ReadFile(programPath)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	if err := os.WriteFile(binaryDst, srcData, 0755); err != nil {
		return fmt.Errorf("write binary to bundle: %w", err)
	}

	// Write Info.plist
	infoPlist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key>
  <string>` + codesignDefaultIdentifier + `</string>
  <key>CFBundleName</key>
  <string>GoGoClaw</string>
  <key>CFBundleExecutable</key>
  <string>gogoclaw</string>
  <key>CFBundleVersion</key>
  <string>1.0</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>LSBackgroundOnly</key>
  <true/>
  <key>LSUIElement</key>
  <true/>
  <key>NSLocalNetworkUsageDescription</key>
  <string>GoGoClaw needs local network access to connect to message queues and other local services.</string>
  <key>NSBonjourServices</key>
  <array>
    <string>_amqp._tcp</string>
  </array>
</dict>
</plist>
`
	if err := os.WriteFile(infoPlistPath, []byte(infoPlist), 0644); err != nil {
		return fmt.Errorf("write Info.plist: %w", err)
	}

	// Sign the bundle
	signArgs := []string{
		"--force", "--deep",
		"--sign", signer,
		"--identifier", codesignDefaultIdentifier,
		"--options", "runtime",
		appDir,
	}
	signOut, err := exec.Command("codesign", signArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sign app bundle: %w: %s", err, strings.TrimSpace(string(signOut)))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created app bundle: %s\n", appDir)
	fmt.Fprintf(cmd.OutOrStdout(), "  Binary:     %s\n", binaryDst)
	fmt.Fprintf(cmd.OutOrStdout(), "  Identifier: %s\n", codesignDefaultIdentifier)
	fmt.Fprintf(cmd.OutOrStdout(), "  Signer:     %s\n", signerDisplay(signer))

	info := codesignInfo(binaryDst)
	fmt.Fprintf(cmd.OutOrStdout(), "  Verify:     %s\n", info)

	fmt.Fprintf(cmd.OutOrStdout(), "\nTo use the bundled binary with the service:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  gogoclaw service install\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  (run from %s)\n", binaryDst)
	return nil
}

func runGrantNetwork(cmd *cobra.Command) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("network permission grant is only supported on macOS")
	}

	programPath, err := os.Executable()
	if err != nil {
		return err
	}
	programPath, err = filepath.EvalSymlinks(programPath)
	if err != nil {
		return err
	}

	// Find the .app bundle by walking up from the binary
	appPath := findAppBundle(programPath)
	if appPath == "" {
		return fmt.Errorf("no .app bundle found; run 'service bundle' first")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Opening %s to trigger local network permission prompt...\n", appPath)
	fmt.Fprintf(cmd.OutOrStdout(), "If a system dialog appears, click 'Allow' to grant local network access.\n\n")

	// Use 'open' to launch the app in GUI context — this triggers TCC prompts
	openOut, err := exec.Command("open", "-a", appPath, "--args", "--config",
		resolveDefaultConfigPath(), "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("open app failed: %w: %s", err, strings.TrimSpace(string(openOut)))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "App opened. After granting permission, restart the service:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  gogoclaw service restart\n")
	return nil
}

func findAppBundle(binaryPath string) string {
	dir := filepath.Dir(binaryPath)
	for dir != "/" && dir != "." {
		if strings.HasSuffix(dir, ".app/Contents/MacOS") || strings.HasSuffix(dir, ".app/Contents/MacOS/") {
			return filepath.Dir(filepath.Dir(dir)) // .app path
		}
		if strings.HasSuffix(dir, ".app") || strings.HasSuffix(dir, ".app/") {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func resolveDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "~/.gogoclaw/config.json"
	}
	return filepath.Join(homeDir, ".gogoclaw", "config.json")
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
	}

	// If the binary is inside a .app bundle, use 'open -W -a' to launch it.
	// This gives the process a GUI application context, which macOS requires
	// for local network access on macOS 15+ (NECP/TCC restrictions).
	appBundle := findAppBundle(programPath)
	if appBundle != "" {
		spec.ProgramArgs = []string{
			"/usr/bin/open", "-W", "-a", appBundle,
			"--args",
			"--config", resolvedConfigPath,
			"gateway",
		}
	} else {
		spec.ProgramArgs = []string{
			programPath,
			"--config",
			resolvedConfigPath,
			"gateway",
		}
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
	content = []byte(strings.ReplaceAll(string(content), "<true></true>", "<true/>"))
	content = []byte(strings.ReplaceAll(string(content), "<false></false>", "<false/>"))
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
