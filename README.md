# YealinkNotifier

A lightweight, cross-platform (Windows / Linux / macOS) notification app for Yealink office phones. It runs a small HTTP server that listens for Yealink "Incoming Call" webhook events and turns them into a **system notification** and a **sound**, with a **system-tray icon** to toggle the notification/sound channels independently, review the last calls, and quit — so you don't miss calls while you're at your desk.

Built as a small Go program with two runtime dependencies: [`beeep`](https://github.com/gen2brain/beeep) (notifications + beep) and [`systray`](https://github.com/getlantern/systray) (the tray menu). Icons are embedded from the [`images/`](images) package.

## How it works

1. You configure a Yealink phone to fire a webhook on incoming calls (see [Webhook setup: Yealink phone](#webhook-setup-yealink-phone)).
2. The phone issues an HTTP request to the YealinkNotifier server.
3. On every request from an allowed source (see [Security](#security)) the app, concurrently:
   - shows a system notification (`Incoming call from Yealink`) via `beeep`, and
   - plays a short beep via `beeep` —
   each channel independently (either can be switched off: "quiet visual only" or "sound only").
4. The event is recorded in the in-memory call history (the 20 most recent).
5. The server immediately replies with `200 OK` (denied requests get `403 Forbidden`).

No configuration files are used — everything is controlled by command-line flags. The only state kept is an **in-memory call history** (the 20 most recent events, lost on restart). The tray provides a **status indicator** (green = at least one channel ON, red = both OFF), a **History** submenu with a **Clear history** item, and an **Exit** item that performs a graceful shutdown. Disabling all channels stops the toast + beep, but the server keeps answering the phone with `200 OK` and still records the call.

## Features

- System notification on each incoming call (`beeep`) — cross-platform
- Short beep on each incoming call (`beeep.Beep`) — cross-platform
- System-tray icon (green = at least one channel on, red = both off), via `systray`
- Independent notification / sound channels — switch either on/off from the tray menu or with the `-notify` / `-beep` flags (e.g. sound-only: `-notify=false`)
- In-memory call history (20 most recent) with a tray **History** submenu and a **Clear history** item
- Optional IP allow-list (`-allowed-ips`) — restrict which clients may trigger the webhook
- Tray **Exit** item (and Ctrl+C / SIGTERM) for graceful shutdown
- Shows the caller number in the notification when provided by the webhook
- Logging to a file, the console, or off — all controlled by flags
- Zero config files — everything is controlled by command-line flags

## Requirements

- **Go 1.27** for building from source
- **cgo (`CGO_ENABLED=1`)** — `systray` and parts of `beeep` require cgo. On Windows this needs a C toolchain (MinGW-w64 / MSYS2 gcc); on Linux `gcc` plus the **GTK 3** and **libayatana-appindicator** dev headers; on macOS the system C toolchain.
- **Runtime (Linux)**: a StatusNotifier/app-indicator tray (DBUS) to render the icon, and a `notify-send` D-Bus session for notifications.
- **Runtime (macOS)**: first-run notification permission may need to be granted.
- **Make** (optional, convenience targets) — on Windows run `make` from Git Bash/MSYS.

## Install

```sh
# 1. Build a native binary (bin/YealinkNotifier); version is injected via -ldflags
make build

# or build directly with go
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o bin/YealinkNotifier .
```

The output is a self-contained binary in [`bin/`](bin).

### Cross-building

| Target | Command | Notes |
|--------|---------|-------|
| Windows | `make build-windows` | → `dist/YealinkNotifier.exe`. From a non-Windows host you need MinGW-w64 (`CC=x86_64-w64-mingw32-gcc`). |
| Linux | `make build-linux` | → `dist/YealinkNotifier`. Needs `gcc` + GTK 3 + app-indicator headers on the build host. |
| macOS | `make build-darwin` | → `dist/YealinkNotifier`. Realistically built on a **macOS host/CI** (cgo + Apple headers), not cross-compiled from Linux. |

## Run

```sh
# default: listen on 0.0.0.0:8080, logs to a file in your user cache dir
go run .
# or
./bin/YealinkNotifier
```

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | TCP port to listen on |
| `-addr` | `0.0.0.0` | Bind address (use `127.0.0.1` to limit exposure) |
| `-logfile` | `<user cache dir>/YealinkNotifier/yealinknotifier.log` | Log file path. Set to an empty string (`-logfile ""`) to log to the console instead |
| `-quiet` | `false` | Disable all logging |
| `-notify` | `true` | Show the system notification (set `-notify=false` for sound-only) |
| `-beep` | `true` | Play the beep (set `-beep=false` for quiet visual only) |
| `-allowed-ips` | _(empty — allow all)_ | Comma-separated client IPs and/or CIDR ranges that may call the webhook (e.g. `10.0.0.5,192.168.1.0/24`); any unparseable entry is a startup error (fail-closed) |
| `-version` | — | Print the version and exit |

Example:

```sh
./bin/YealinkNotifier -port 9000 -addr 127.0.0.1
# or: log to the console and disable the tray-less file log
./bin/YealinkNotifier -logfile ""
# or: start without any logging
./bin/YealinkNotifier -quiet
# or: quiet visual only (no beep), restricted to the office subnet
./bin/YealinkNotifier -beep=false -allowed-ips 192.168.1.0/24
```

### Logging

Logging goes through the standard `log` package. By default it is written to a file under your per-user cache / local-application-data directory (e.g. `~/.cache/YealinkNotifier/yealinknotifier.log`, `%LOCALAPPDATA%\YealinkNotifier\yealinknotifier.log`, `~/Library/Caches/YealinkNotifier/yealinknotifier.log`), because a tray/console-less build has no terminal to write to. Use `-logfile ""` to send it to the console (useful when running from a terminal on Linux/macOS) or `-quiet` to turn it off entirely. The log records the startup banner (version, port, webhook URLs), each webhook event, notification/beep errors, tray toggle changes, and the graceful-shutdown sequence.

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

The app reads exactly one parameter (`number`, falling back to `caller`). Allowed callers get `200 OK`; when you set up an IP allow-list, other sources get `403 Forbidden` (see [Security](#security)).

## API

| Method & path | Query | Response |
|---------------|-------|----------|
| `GET /` (any method — the handler is method-agnostic) | `number` *or* `caller` (optional) | `200 OK`, body `OK`; `403 Forbidden`, body `Forbidden` when rejected by `-allowed-ips` |

The caller value is used to populate the notification message:

- with a caller → `Caller number: 123456`
- without → `You are being called! Check your office phone.`

Every accepted event is also recorded in the in-memory call history (sanitized caller, or `unknown` when absent) and shown in the tray **History** submenu.

## Make targets

| Target | Description |
|--------|-------------|
| `make all` / `make build` | Build the native binary into `bin/` (`CGO_ENABLED=1`, version injected via `-ldflags main.version`) |
| `make build-windows` / `make build-linux` / `make build-darwin` | Cross-builds into `dist/` (see cross-building notes) |
| `make run` | Run from source with `go run .` |
| `make vet` | `go vet ./...` |
| `make fmt` | `gofmt -w .` |
| `make test` | `go test ./...` — runs the unit-test suite (`core` + `server`) |
| `make clean` | Remove `bin/` and `dist/` |

> On Windows, run `make` from Git Bash/MSYS. The `clean` target uses POSIX `rm -rf`.

## Design notes

- **Cross-platform by design.** Notifications and sound go through `beeep`; the tray goes through `systray`. No Windows-only `syscall`/`winmm`/PowerShell code remains. `beeep.Beep` and notifications are best-effort per platform (they may be no-ops in some environments), but they never block the request.
- **Independent channels.** The system notification and the beep are separate channels with independent state (`core.New(notify, beep)`, `-notify` / `-beep` flags, tray checkboxes). `OnCall` dispatches each enabled channel in its own goroutine, so neither can block the request.
- **Tray status, history & graceful shutdown.** The tray icon reflects channel state (green if at least one channel is on, red if both are off; tooltip `YealinkNotifier — notifications ON/OFF`). The menu has a master toggle, independent "Show notifications" / "Play sound" checkboxes, a **History** submenu (20 most recent, newest first) and **Clear history**. `Exit` (or Ctrl+C / SIGTERM) shuts the HTTP server down (5s drain) and then quits the tray. If the tray can't start (e.g. headless Linux, no display), the app degrades to a notification-only server instead of failing.
- **Light state by design.** No persistence, no auth — the call history is in-memory only (20 most recent events, lost on restart). The webhook URL must be reachable only from trusted networks (or restricted via `-allowed-ips`); don't expose it to the internet.

## Security

The primary network control is the optional **IP allow-list** (`-allowed-ips`): a comma-separated list of client IPs and/or CIDR ranges, IPv4 and IPv6 alike (e.g. `-allowed-ips 10.0.0.5,192.168.1.0/24`). Left empty (the default) all clients are allowed — the previous behavior. Notes:

- Parsing is **fail-closed**: any unparseable entry is a startup error (exit 1) *before* the socket opens.
- Matching uses only the TCP peer address (`r.RemoteAddr`) — **no `X-Forwarded-For`** header is trusted — so behind a proxy the allow-list sees the proxy's IP. Put YealinkNotifier on the phone's direct path, not behind a proxy.
- Denied requests get `403 Forbidden` and are **not** recorded in the call history.

Otherwise this is an unauthenticated endpoint meant for local/office networks. If you must bind more than `127.0.0.1`, keep the port closed or protected at the network level — any host that can reach it can trigger a notification + beep on the machine.

The caller value from the webhook is sanitized before use (SIP URIs such as `sip:144@10.0.0.1` are reduced to the number, a trailing `_<suffix>` part is stripped, XML-unsafe characters are removed, length capped) so a rogue webhook cannot bloat the notification or inject markup into the Windows toast.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
