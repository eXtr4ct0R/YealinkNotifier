package tray

import (
	"log"
	"runtime"
	"sync/atomic"

	"github.com/getlantern/systray"

	"YealinkNotifier/core"
	"YealinkNotifier/images"
)

var (
	toggle  *systray.MenuItem
	started atomic.Bool
)

func trayIconBytes(on bool) []byte {
	windows := runtime.GOOS == "windows"
	if on {
		if windows {
			return images.IcoOnData
		}
		return images.PngOnData
	}
	if windows {
		return images.IcoOffData
	}
	return images.PngOffData
}

func Stop() {
	if started.Load() {
		systray.Quit()
	}
}

func onReady(app *core.App, shutdown func()) {
	started.Store(true)
	systray.SetIcon(trayIconBytes(true))
	systray.SetTooltip("YealinkNotifier — notifications ON")

	toggle = systray.AddMenuItem("Disable notifications", "Enable or disable incoming-call notifications")
	toggle.Check()
	go func() {
		for range toggle.ClickedCh {
			on := !app.Enabled()
			app.SetEnabled(on)
			if on {
				systray.SetIcon(trayIconBytes(true))
				systray.SetTooltip("YealinkNotifier — notifications ON")
				toggle.Check()
				toggle.SetTitle("Disable notifications")
				log.Printf("Notifications enabled")
			} else {
				systray.SetIcon(trayIconBytes(false))
				systray.SetTooltip("YealinkNotifier — notifications OFF")
				toggle.Uncheck()
				toggle.SetTitle("Enable notifications")
				log.Printf("Notifications disabled")
			}
		}
	}()

	exitItem := systray.AddMenuItem("Exit", "Exit YealinkNotifier")
	go func() {
		for range exitItem.ClickedCh {
			Stop()
			shutdown()
		}
	}()
}

func onExit(shutdown func()) {
	shutdown()
}

func RunSafe(app *core.App, shutdown func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("tray unavailable (%v); continuing as notification-only server", r)
		}
	}()
	systray.Run(func() { onReady(app, shutdown) }, func() { onExit(shutdown) })
	<-app.Done()
}
