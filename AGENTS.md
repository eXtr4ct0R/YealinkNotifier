# AGENTS.md

YealinkNotifier is a **Windows-only, single-file Go app** (`main.go`): an HTTP server that receives Yealink phone "Incoming Call" webhooks and shows a Windows toast notification + system sound. Go 1.27, module `YealinkNotifier`, one direct dependency (`go-toast`, with a PowerShell fallback already wired in).

## Commands

- Run: `make run` (=`go run .`) or `go run . -port 8080 -addr 0.0.0.0`. Flags: `-port` (default `8080`), `-addr` (default `0.0.0.0`), `-version` (print version and exit)
- Verify: `make vet` (`go vet ./...`) plus a format check — `gofmt -l .` (lists files needing formatting; empty = clean) or `make fmt` (auto-fixes). There is **no test suite**: `make test` is a no-op placeholder, don't look for tests
- Build: `make build` → `bin/YealinkNotifier`. Built with `-trimpath -ldflags "-s -w -X main.version=$(VERSION)"`, where `VERSION = git describe --tags --always --dirty || dev`. Binaries in `bin/` are gitignored and pre-existing — never commit or delete them

## Conventions (do NOT change)

- **Windows-only is intentional.** Direct `syscall` + `winmm.dll` (`PlaySoundW`, played with `SND_ASYNC|SND_FILENAME`) for the sound, `go-toast` for the toast, and a PowerShell `Windows.UI.Notifications` fallback throttled to 3 concurrent via the `fallbackSem` semaphore. Don't abstract these away, "port to Linux", or remove the fallback
- The sound file is **hardcoded**: `C:\Windows\Media\Windows Notify System Generic.wav`, with `C:\Windows\Media\Windows Notify.wav` as fallback. The toast `AppID` is `"YealinkNotifier"`. Don't make either config-driven
- User-facing strings, toast text, and log messages are in **English** — keep new strings in English too
- Webhook contract: handle **any HTTP method** on `/`; read query `number` (Yealink sends `$remote_number`), falling back to `caller`; respond `200 OK` with body `OK`. Keep it simple and stateless — no auth, no persistence, no call history
- Code comments are in English; match local style, don't translate

## Gotchas

- `make` targets use POSIX shell (`rm -rf`, `$$` vars) — on Windows run make from Git Bash/MSYS, not raw make-for-Windows, or the `clean` target will fail
- `go.mod` says `go 1.27` — use a matching local toolchain
- The server binds `0.0.0.0` by default with **no authentication**. This is an unauthenticated endpoint meant for local/LAN use only — don't expose it to the internet; guard the port at the network level if it must be reachable beyond localhost
- The handler stays non-blocking: `showToast` and `playWindowsSound` each run in their own goroutine, and the response is written to the phone immediately. Preserve this
