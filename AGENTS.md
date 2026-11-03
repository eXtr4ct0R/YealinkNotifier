# AGENTS.md

YealinkNotifier is a **cross-platform (Windows / Linux / macOS), small Go app**: an HTTP server that receives Yealink phone "Incoming Call" webhooks and shows a **system notification + beep**, with a **system-tray icon** (status toggle + Exit for graceful shutdown). Go 1.27, module `YealinkNotifier`.

## Layout

- `main.go` — flags, `log` setup, the startup banner, the composition root (wires `core` + `server` + `tray`), the SIGINT/SIGTERM watcher, and the bounded (~5s) shutdown closure.
- `core/` — `App`: the notification on/off state (`atomic.Bool`), `OnCall` (spawns `Notify` + `Beep` in separate goroutines, sanitizes the caller), `Notify`/`Beep` via `beeep`, and the shutdown orchestration (`Shutdown` is `sync.Once`-deduped, `Done()` closes after the server drain). `beep_windows.go`/`beep_other.go` — beep fallback on Windows (`MessageBeep` when `beeep.Beep` fails) / passthrough log on other OSes.
- `server/` — the `http.Server` (full timeout set) + the method-agnostic `/` handler (query `number` with `caller` fallback) + bounded `Shutdown`.
- `tray/` — the `systray` menu: status toggle (on/off icon), Exit, `Stop()`, the headless-safe `RunSafe` wrapper, and the per-OS `trayIconBytes` selector (ICO on Windows, PNG on Linux/macOS).
- `images/` — embedded icons: PNG (`PngOnData` green / `PngOffData` red / `PngIncomingData` for `beeep.Notify`) for the Linux/macOS tray + notifications, ICO (`IcoOnData`/`IcoOffData`) for the Windows tray.
- A small set of packages (`main`, `core`, `server`, `tray`, `images`) with a one-way dependency graph `main → {core, server, tray, images}`; no frameworks, no further abstraction layers. Keep it that way.

## Commands

- Run: `make run` (=`go run .`) or the built binary. Flags: `-port` (default `8080`), `-addr` (default `0.0.0.0`), `-logfile` (default: a file under the OS user-cache dir; empty string = console), `-quiet` (disable logging), `-version` (print version and exit).
- Verify: `make vet` (`go vet ./...`) plus a format check — `gofmt -l .` (lists files needing formatting; empty = clean) or `make fmt` (auto-fixes). There is **no test suite**: `make test` is a no-op placeholder, don't look for tests.
- Build: `make build` (native) → `bin/YealinkNotifier`; cross-builds `make build-windows` / `make build-linux` / `make build-darwin` → `dist/`. All builds are **`CGO_ENABLED=1`** (see gotchas). Built with `-trimpath -ldflags "-s -w -X main.version=$(VERSION)"`. Binaries in `bin/`/`dist/` are gitignored and pre-existing — never commit or delete them.

## Conventions (do NOT change)

- **Cross-platform is the point.** Do **not** reintroduce platform-specific paths in shared code (`winmm.dll`/`PlaySoundW`, PowerShell, `go-toast`). Notifications+sound go through `github.com/gen2brain/beeep`; the tray goes through `github.com/getlantern/systray`. The **only** sanctioned platform exceptions are the two build-tagged files: `core/beep_windows.go` (Windows `MessageBeep` fallback via `syscall`, see gotchas) and `trayIconBytes` in `tray/tray.go` (ICO vs PNG selection). Add new cross-platform behavior without breaking the win/linux/darwin builds.
- **Tray semantics:** icon = notification state (green `PngOnData` = ON, red `PngOffData` = OFF). The tray menu has a notifications toggle and an **Exit** item. When notifications are OFF the server must still answer `200 OK`; only the toast/beep are suppressed.
- **Graceful shutdown:** `Exit` / SIGINT / SIGTERM must end the tray loop, drain the HTTP server (bounded, ~5s), and only then let the process exit — the wait is enforced by `tray.RunSafe` blocking on `app.Done()`. `systray.Quit()` is only ever called from background goroutines (signal watcher, Exit item) — **never from the `systray` event-loop (`onExit`)**, because on macOS `systray.Quit()` blocks until the run loop finishes terminating (that is what fires `onExit`), so calling it there deadlocks.
- The tray code must be **headless-safe**: if the tray can't start (no display / no DBUS), recover and continue as a notification-only server rather than crashing.
- **Icons:** the embedded icons in `images/` are the single source. **Windows tray** must use the ICO bytes (`IcoOnData`/`IcoOffData`) — systray's Win32 `LoadImage` only reads `.ico` files (PNG fails silently with the bogus "The operation completed successfully."); **Linux/macOS tray** use the PNG bytes; **`beeep.Notify`** keeps the PNG bytes (the `PngIncomingData` icon; the Windows tray is the only ICO consumer.). Select per-OS via the `trayIconBytes` helper in `tray/tray.go`. Do not add icon tooling.
- User-facing strings, tray labels, and log messages are **English** — keep new strings in English. User-facing (non-log) text is the notification `Incoming call from Yealink` / `Caller number: %s` (fallback when no caller: `You are being called! Check your office phone.`) — preserve it.
- Webhook contract: handle **any HTTP method** on `/`; read query `number` (Yealink sends `$remote_number`), falling back to `caller`; respond `200 OK` with body `OK`. The caller value is sanitized (XML-unsafe chars stripped, length capped) before it is used in the notification — preserve this. Keep it simple and stateless — no auth, no persistence, no call history.
- Code comments are in English; match local style.

## Logging

- Route all runtime output through the standard `log` package (never `fmt.Println` for operational messages). The startup banner, webhook events, and any errors are logged.
- `setupLogging(logfile, quiet)` selects the sink: `quiet` → `io.Discard`; `logfile == ""` → console (stderr); otherwise an append-mode file (created, dir auto-created). On GUI/console-less Windows builds the file is the realistic destination.

## Gotchas

- **`systray` (and parts of `beeep`) require cgo — builds must be `CGO_ENABLED=1`.** The Makefile sets this. A cgo build needs the platform C/toolchain + dev headers: Windows → MinGW-w64/MSYS2 gcc; Linux → `gcc` + **GTK 3** + **libayatana-appindicator** dev headers; macOS → system C toolchain.
- **Cross-build reality:** `build-windows` from a non-Windows host needs `CC=x86_64-w64-mingw32-gcc`; `build-darwin` is realistically built on a **macOS host/CI**, not cross-compiled from Linux. Don't claim one-from-all.
- **Linux runtime:** the tray icon needs a StatusNotifier/app-indicator (DBUS) environment; notifications use a `notify-send` D-Bus session. `beeep.Beep` and `beeep.Notify` are best-effort and may be no-ops in some environments — they must never block the HTTP request.
- **Windows beep quirk:** `beeep.Beep` uses `kernel32.Beep`, which works only in console-subsystem processes. The shipped binary is `-H windowsgui` (no console), so it fails with the bogus "The handle is invalid." (verified standalone). `beep_windows.go` falls back to `user32.MessageBeep` (MB_OK) when `beeep.Beep` returns an error. Console builds (`go run`) beep directly — the fallback simply never fires there. Don't "simplify" the fallback away.
- The server binds `0.0.0.0` by default with **no authentication**. This is an unauthenticated endpoint meant for local/LAN use only — don't expose it to the internet; guard the port at the network level if it must be reachable beyond localhost.
- The handler stays non-blocking: `notify` and `beep` each run in their own goroutine, and the response is written to the phone immediately. Preserve this.
- `make` targets use POSIX shell — on Windows run make from Git Bash/MSYS, not raw make-for-Windows, or the `clean` target (`rm -rf`) will fail.
- `go.mod` says `go 1.27` — use a matching local toolchain.
