package core

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/beeep"

	"YealinkNotifier/images"
)

type App struct {
	enabled  atomic.Bool
	shutdown func(context.Context)

	stop     chan struct{}
	stopOnce sync.Once
}

func New() *App {
	a := &App{stop: make(chan struct{})}
	a.enabled.Store(true)
	return a
}

func (a *App) Enabled() bool {
	return a.enabled.Load()
}

func (a *App) SetEnabled(on bool) {
	a.enabled.Store(on)
}

func (a *App) OnCall(caller string) {
	if !a.Enabled() {
		return
	}
	caller = sanitizeCaller(caller)
	title := "Incoming call from Yealink"
	message := "You are being called! Check your office phone."
	if caller != "" {
		message = fmt.Sprintf("Caller number: %s", caller)
	}
	go a.Notify(title, message)
	go a.Beep()
}

func sanitizeCaller(caller string) string {
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
