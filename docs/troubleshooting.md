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