package core

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/beeep"

	"YealinkNotifier/images"
)

type App struct {
	notify   atomic.Bool
	beep     atomic.Bool
	shutdown func(context.Context)

	histMu  sync.Mutex
	hist    []Event
	histVer atomic.Uint64

	stop     chan struct{}
	stopOnce sync.Once
}

func New(notify, beep bool) *App {
	a := &App{stop: make(chan struct{})}
	a.notify.Store(notify)
	a.beep.Store(beep)
	return a
}

func (a *App) NotifyEnabled() bool {
	return a.notify.Load()
}

func (a *App) SetNotify(on bool) {
	a.notify.Store(on)
}

func (a *App) BeepEnabled() bool {
	return a.beep.Load()
}

func (a *App) SetBeep(on bool) {
	a.beep.Store(on)
}

func (a *App) AnyEnabled() bool {
	return a.notify.Load() || a.beep.Load()
}

func (a *App) OnCall(caller string) {
	caller = sanitizeCaller(caller)
	a.RecordEvent(caller)
	title := "Incoming call from Yealink"
	message := "You are being called! Check your office phone."
	if caller != "" {
		message = fmt.Sprintf("Caller number: %s", caller)
	}
	if a.NotifyEnabled() {
		go a.Notify(title, message)
	}
	if a.BeepEnabled() {
		go a.Beep()
	}
}

var (
	sipURIRe = regexp.MustCompile(`(?i)^sips?:([^\s:@/?#;]+)((;[^\s@/?#;]*)*)@`)
	suffixRe = regexp.MustCompile(`_[A-Za-z][A-Za-z0-9]*$`)
)

func sanitizeCaller(caller string) string {
	caller = strings.TrimSpace(caller)
	// "sip:228@10.178.34.88" -> "228"
	if m := sipURIRe.FindStringSubmatch(caller); m != nil {
		caller = m[1]
	}
	// "144_number" -> "144"
	caller = suffixRe.ReplaceAllString(caller, "")
	caller = strings.NewReplacer("<", "", ">", "", "&", "", `"`, "", "'", "").Replace(caller)
	runes := []rune(caller)
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return string(runes)
}

func (a *App) Notify(title, message string) {
	if err := beeep.Notify(title, message, images.PngIncomingData); err != nil {
		log.Printf("Error sending notification: %v", err)
	}
}

func (a *App) Beep() {
	if err := beeep.Beep(beeep.DefaultFreq, 300); err != nil {
		beepFallback(err)
	}
}

func (a *App) SetShutdown(fn func(context.Context)) {
	a.shutdown = fn
}

func (a *App) Shutdown(ctx context.Context) {
	a.stopOnce.Do(func() {
		if a.shutdown != nil {
			a.shutdown(ctx)
		}
		close(a.stop)
	})
}

func (a *App) Done() <-chan struct{} {
	return a.stop
}
