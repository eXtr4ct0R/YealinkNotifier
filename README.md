# YealinkNotifier

A lightweight, cross-platform (Windows / Linux / macOS) Go app for Yealink office phones: an HTTP server that turns "Incoming Call" webhook events into a **system notification** and a **sound**, with a **system-tray icon** to toggle the channels, review the last calls, and quit — so you don't miss calls at your desk.

Runtime dependencies: [`beeep`](https://github.com/gen2brain/beeep) (notifications + beep), [`systray`](https://github.com/getlantern/systray) (tray), [`yaml.v3`](https://gopkg.in/yaml.v3) (optional config file). Icons are embedded from the [`images/`](images) package.

## How it works

1. The Yealink phone fires a webhook on incoming calls ([setup](#webhook-setup-yealink-phone)).
2. On each request from an allowed source the app, concurrently, shows a notification and plays a beep — each channel independently switchable — and records the event in the in-memory history (last 20).
3. The server immediately replies `200 OK` (denied: `403 Forbidden`) — even when both channels are off.

Configuration: an **optional [YAML config file](#configuration-file-yaml)**, **command-line flags as overrides** (highest priority), built-in defaults as the last resort — no config file still works. The only state is the **in-memory call history** (lost on restart).

## Features

- System notification + beep per call (`beeep`) — cross-platform; each channel independently on/off (tray menu, config file, or flags — e.g. sound-only: `-notify=false`)
- Tray icon (green = at least one channel on, red = both off), **History** submenu (last 20) + **Clear history**, **Exit** (Ctrl+C / SIGTERM too) for graceful shutdown
- Caller number shown in the notification when the webhook provides one
- Optional IP allow-list — restrict which clients may trigger the webhook
- Logging to a file, the console, or off (config file and/or flags)
- Optional `config.yml` — no config file required at all

## Requirements

- **Go 1.27**; builds are **cgo (`CGO_ENABLED=1`)** — Windows: MinGW-w64/MSYS2 gcc; Linux: `gcc` + **GTK 3** + **libayatana-appindicator** headers; macOS: system C toolchain
- **Linux runtime**: StatusNotifier/app-indicator (DBUS) tray + `notify-send` session; **macOS runtime**: first-run notification permission
- **Make** (optional) — on Windows run it from Git Bash/MSYS

## Install

```sh
make build    # → bin/YealinkNotifier (CGO_ENABLED=1, version via -ldflags)
# or: CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o bin/YealinkNotifier .
```

| Cross-build | Command | Notes |
|-------------|---------|-------|
| Windows | `make build-windows` | → `dist/`. From non-Windows: `CC=x86_64-w64-mingw32-gcc` |
| Linux | `make build-linux` | Needs `gcc` + GTK 3 + app-indicator headers |
| macOS | `make build-darwin` | Realistically built on a **macOS host/CI**, not from Linux |

## Run

```sh
go run .            # or: ./bin/YealinkNotifier
./bin/YealinkNotifier -port 9000 -addr 127.0.0.1 -beep=false -allowed-ips 192.168.1.0/24
```

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | TCP port to listen on |
| `-addr` | `0.0.0.0` | Bind address (`127.0.0.1` to limit exposure) |
| `-logfile` | `<user cache dir>/…/yealinknotifier.log` | Log path; `-logfile ""` = console |
| `-quiet` | `false` | Disable all logging |
| `-notify` | `true` | Show the system notification |
| `-beep` | `true` | Play the beep |
| `-allowed-ips` | _(empty — allow all)_ | Comma-separated IPs/CIDRs allowed to call the webhook; any unparseable entry = startup error (fail-closed) |
| `-config` | _(empty — auto-discover)_ | Path to a YAML config file; see [Configuration file](#configuration-file-yaml) |
| `-version` | — | Print the version and exit |

### Configuration file (YAML)

All keys optional; a ready-to-use commented example ships in [`config.example.yaml`](config.example.yaml):

```yaml
port: 8080                      # default: 8080
addr: 0.0.0.0                   # default: 0.0.0.0
notify: true                    # default: true (system notification)
beep: true                      # default: true (sound)
allowed_ips: [127.0.0.1, 192.168.1.0/24]   # empty/omitted = allow all
logfile: ""                     # "" = console; omitted = file under the user cache dir
quiet: false                    # default: false
```

- **Finding the file.** Without `-config`: `config.yml` next to the executable → in the working directory → in the user config dir (`%APPDATA%` win, `$XDG_CONFIG_HOME`/`~/.config` linux, `~/Library/Application Support` macOS); first existing wins, none = defaults + warning. With `-config <path>` that file must exist (a miss is a startup error).
- **Priority.** Per parameter, independently: **explicit flag > config file > default** — a flag wins by *presence*, so `-port=8080` still overrides `port: 9000`.
- **Fail-closed.** The file is read-only, read once at startup. Unparseable YAML, an unknown key (`allowedIps`), or a wrong-typed key (`port: "eighty"`) is a startup error (exit 1) *before* the socket opens. Empty/`{}` files are valid (= defaults). `logfile: ""` (empty *value*) is meaningful — console — while an *absent* key falls to the default.

### Logging

By default the log goes to a file under the per-user cache dir (`~/.cache/…`, `%LOCALAPPDATA%\YealinkNotifier\…`, `~/Library/Caches/…`) — a tray build has no terminal. Use `-logfile ""` / `logfile: ""` for the console, or `-quiet` / `quiet: true` to turn logging off.

## Webhook setup: Yealink phone

In the phone's incoming-call / DND **Webhook Settings** (or under `Admin`/`Network`, depending on firmware), set the **URL** to:

```
http://<your-machine-ip>:8080/?number=$remote_number
```

- Query: `number` (the `$remote_number` macro; recommended) or the alias `caller`; omit both and the app still notifies, without the caller.
- Caller shown as `Caller number: 123456`; without one → `You are being called! Check your office phone.`
- The handler is method-agnostic (`GET /` and friends); allowed callers get `200 OK` / `OK`, rejected ones `403 Forbidden` / `Forbidden`.

## Make targets

| Target | Description |
|--------|-------------|
| `make build` | Native binary into `bin/` (`CGO_ENABLED=1`, version via `-ldflags`) |
| `make build-{windows,linux,darwin}` | Cross-builds into `dist/` (see notes above) |
| `make run` | `go run .` |
| `make vet` / `make fmt` | `go vet ./...` / `gofmt -w .` |
| `make test` | `go test ./...` (suite: `config` + `core` + `server`) |
| `make clean` | Remove `bin/` and `dist/` (POSIX `rm -rf` → Git Bash/MSYS on Windows) |

## Design & security notes

- **Independent, non-blocking channels.** Notification and beep are separate states, each dispatched in its own goroutine; both are best-effort (may no-op on some platforms) but never block the request.
- **Headless-safe.** No display/DBUS → the app degrades to a notification-only server instead of crashing.
- **Light state.** No persistence, no auth — history is in-memory (20, lost on restart); the config file is read-only (never created or written).
- **LAN-only by design.** The unauthenticated endpoint binds `0.0.0.0` by default. The IP allow-list (`-allowed-ips` / `allowed_ips`, flag wins) parses fail-closed, matches the **TCP peer only** (`r.RemoteAddr`, no `X-Forwarded-For` — behind a proxy it sees the proxy's IP), and denied requests are not recorded. Webhook caller values are sanitized (SIP URI → number, `_<suffix>` stripped, XML-unsafe chars removed, length capped). Keep the port closed to untrusted networks.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
