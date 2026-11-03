# YealinkNotifier

A lightweight, Windows-only notification app for Yealink office phones. It runs a small HTTP server that listens for Yealink "Incoming Call" webhook events and turns them into a **Windows toast notification** plus a **system sound**, so you don't miss calls while you're working at your desk.

Built as a single Go source file (`main.go`) with a single runtime dependency (`go-toast`, with a built-in PowerShell fallback).

## How it works

1. You configure a Yealink phone to fire a webhook on incoming calls (see [Webhook setup: Yealink phone](#webhook-setup-yealink-phone)).
2. The phone issues an HTTP request to the YealinkNotifier server.
3. On every request the app, concurrently:
   - shows a Windows toast notification (`📞 Incoming call from Yealink`), and
   - plays a system notification sound asynchronously.
4. The server immediately replies with `200 OK`.

The app is stateless — each call event is independent, no call history is kept, and no configuration files are used.

## Features

- 📞 Windows toast notification on each incoming call
- 🔊 Asynchronous system sound (Windows `Notify` sound via `winmm.dll`, `PlaySoundW`)
- 🔁 Automatic PowerShell fallback if the primary toast path fails
- 📇 Shows the caller number in the notification when it is provided by the webhook
- ⚙️ Zero config files — everything is controlled by command-line flags
- 📦 Single-file app, single dependency, no database

## Requirements

- **Windows** (toast notifications + `winmm.dll` are Windows-specific by design)
- **Go 1.27** for building from source
- **Make** (optional, convenience targets) — on Windows use Git Bash/MSYS for `make`

## Install

```sh
# 1. Build a standalone binary (bin/YealinkNotifier)
make build

# or build directly with go
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/YealinkNotifier .
```

The output is a self-contained binary in [`bin/`](bin).

## Run

```sh
# default: listen on 0.0.0.0:8080
go run .
# or
./bin/YealinkNotifier
```

On startup the app prints the server info and the recommended Yealink webhook URLs:

```
==================================================
   Version: vX.Y.Z
   Server listening on port: 8080
   URL for Yealink Webhook:
   http://0.0.0.0:8080/?number=$remote_number
   or http://0.0.0.0:8080/?number=%remote_number%
   or http://0.0.0.0:8080/
==================================================
```

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | TCP port to listen on |
| `-addr` | `0.0.0.0` | Bind address (use a specific IP to limit exposure) |
| `-version` | — | Print the version and exit |

Example:

```sh
./bin/YealinkNotifier -port 9000 -addr 127.0.0.1
```

## Webhook setup: Yealink phone

On the Yealink phone, navigate to the incoming-call / "DND" → **Webhook Settings** (or the general Webhook settings under `Admin` / `Network` depending on the firmware), and set the **URL** to the YealinkNotifier endpoint:

```
http://<your-machine-ip>:8080/?number=$remote_number
```

Replace `<your-machine-ip>` with the IP of the machine running YealinkNotifier (use the LAN address, not `0.0.0.0`).

Notes on the query string:

- `number` — recommended; matches the Yealink `$remote_number` macro the phone already substitutes.
- `caller` — supported as an alias; used automatically if `number` is absent.
- Omit the query entirely (`http://<your-machine-ip>:8080/`) and the app still notifies, just without the caller number.

The app reads exactly one parameter (`number`, falling back to `caller`) and always returns `200 OK`.

## API

| Method & path | Query | Response |
|---------------|-------|----------|
| `GET /` (any method — the handler is method-agnostic) | `number` *or* `caller` (optional) | `200 OK`, body `OK` |

The caller value is only used to populate the notification message:

- with a caller → `Caller number: 123456`
- without → `You are being called! Check your office phone.`

## Make targets

| Target | Description |
|--------|-------------|
| `make all` / `make build` | Build the binary into `bin/` (version injected via `-ldflags main.version`) |
| `make run` | Run from source with `go run .` |
| `make vet` | `go vet ./...` |
| `make fmt` / `make fmt-check` | Format / check formatting |
| `make test` | `go test ./...` (placeholder — the project intentionally has no test suite) |
| `make clean` | Remove `bin/` and `dist/` |

> On Windows, run `make` from Git Bash/MSYS. The `clean` target uses POSIX `rm -rf`.

## Design notes

- **Windows-only is intentional.** The app calls `syscall` + `winmm.dll` (`PlaySoundW`) directly, uses `go-toast` for notifications, and has a PowerShell fallback (limited to 3 concurrent by a semaphore). Don't abstract these away or "port to Linux" — the platform assumptions are load-bearing.
- **Stateless by design.** No persistence, no auth. The webhook URL must be reachable only from trusted networks; don't expose it to the internet.

## Security

This is an unauthenticated HTTP endpoint meant for local/office networks. If you must bind more than `127.0.0.1`, keep the port closed or protected at the network level — any host that can reach it can trigger a toast + sound on the machine.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
