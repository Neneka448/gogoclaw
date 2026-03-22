# Troubleshooting

## Ubuntu build error: SQLiteConn.LoadExtension undefined

If you build the project on Ubuntu and see an error similar to this:

```text
internal/vectorstore/sqlite_vec_service.go:276:22: sqliteConn.LoadExtension undefined (type *sqlite3.SQLiteConn has no field or method LoadExtension)
```

the usual cause is that cgo is disabled during the build.

### Why it happens

This project uses [github.com/mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) together with the sqlite-vec loadable extension. Extension loading relies on cgo and native SQLite integration. When `CGO_ENABLED=0`, the SQLite driver is compiled without the extension-loading support needed by `LoadExtension`.

### How to confirm

Check your Go environment:

```bash
go env CGO_ENABLED
go env GOOS GOARCH CC CXX
```

If `CGO_ENABLED` is `0`, that is the reason for this build failure.

### Fix

Install a native build toolchain if needed:

```bash
sudo apt update
sudo apt install -y build-essential gcc g++ libc6-dev pkg-config
```

Then build with cgo enabled:

```bash
CGO_ENABLED=1 make build
CGO_ENABLED=1 make test
```

You can also build directly with Go:

```bash
CGO_ENABLED=1 go build -o gogoclaw .
```

### Make the fix persistent

If your shell or environment sets `CGO_ENABLED=0`, remove that override or change it:

```bash
grep -R "CGO_ENABLED" -n ~/.bashrc ~/.profile ~/.zshrc /etc/profile /etc/environment 2>/dev/null
go env -w CGO_ENABLED=1
```

### Notes

- This is a compile-time issue, not a missing runtime `.so` issue.
- Matching the repository's `go-sqlite3` version alone is not enough if cgo is disabled.
- The sqlite-vec integration in this repository is not expected to work with `CGO_ENABLED=0`.

---

## macOS: launchd service cannot connect to local network ("no route to host")

If gogoclaw runs fine in the foreground but the launchd service fails with:

```text
Error: dial tcp <ip>:5672: connect: no route to host
```

this is caused by macOS 15+ NECP/TCC restrictions that block background daemon processes from accessing the local network.

### Root cause

macOS 15 and later enforce a Local Network permission (TCC) for applications. Processes launched directly by launchd as bare binaries lack the GUI application context that macOS requires to grant (or even prompt for) local network access. This affects connections to local services like RabbitMQ, Redis, or any LAN host.

The same binary works fine when run from Terminal because foreground shell processes inherit the terminal's network permissions.

### Fix: use an .app bundle with `open -W -a`

The recommended workflow:

```bash
# 1. Build the binary (auto-signs on macOS)
make build

# 2. Create a .app bundle with Info.plist (includes NSLocalNetworkUsageDescription)
./gogoclaw service bundle

# 3. Install the launchd service from the bundled binary
#    This auto-detects the .app bundle and uses 'open -W -a' in the plist
~/Applications/GoGoClaw.app/Contents/MacOS/gogoclaw service install
```

The generated launchd plist will use `/usr/bin/open -W -a GoGoClaw.app` instead of calling the binary directly. This gives the process a GUI application context that macOS allows through NECP.

### Optional: stronger signing identity

Ad-hoc signing (the default) is usually sufficient. If you use a VPN or proxy tool (e.g. Clash) that applies per-app network rules, a proper signing certificate may help:

```bash
# Create a self-signed code signing certificate
./gogoclaw service setup-cert

# Rebuild the bundle with the certificate
./gogoclaw service bundle --signer "GoGoClaw Code Signing"

# Reinstall the service
~/Applications/GoGoClaw.app/Contents/MacOS/gogoclaw service install
```

### Verify

Check the service status and signing info:

```bash
~/Applications/GoGoClaw.app/Contents/MacOS/gogoclaw service status
```

Check for active MQ connections:

```bash
lsof -p $(pgrep -f gogoclaw) 2>/dev/null | grep -E 'TCP.*amqp|TCP.*5672'
```

### Key diagnostic commands

```bash
# Check if the binary is signed
codesign -dvv ./gogoclaw

# Check Gatekeeper assessment
spctl --assess --type execute ./gogoclaw

# Check launchd plist uses 'open'
cat ~/Library/LaunchAgents/com.gogoclaw.gateway.*.plist | grep -A2 ProgramArguments
```

### Notes

- This issue only affects macOS 15+ (including macOS 26). Older macOS versions and Linux are not affected.
- The `service bundle` command creates `~/Applications/GoGoClaw.app` by default. Use `--dir` to change the location.
- The `.app` bundle includes `LSBackgroundOnly=true` so it does not appear in the Dock.
- On Linux, `service install` creates a standard launchd-free service and no signing or bundling is needed.