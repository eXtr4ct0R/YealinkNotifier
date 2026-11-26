package tray

import (
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/getlantern/systray"

	"YealinkNotifier/core"
	"YealinkNotifier/images"
)

var (
	toggle    *systray.MenuItem
	showItem  *systray.MenuItem
	soundItem *systray.MenuItem
	started   atomic.Bool
)

const (
	// contactsMenuCapacity is the number of pre-created Contacts submenu
	// slots; entries past it are truncated silently.
	contactsMenuCapacity = 25
	// noContactsLabel is the slot-0 placeholder of an empty contacts book.
	noContactsLabel = "No contacts"
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

func renderHistory(slots []*systray.MenuItem, app *core.App) {
	events := app.Events()
	for i, slot := range slots {
		if i < len(events) {
			slot.SetTitle(events[i].Time.Format("15:04:05") + " — " + app.Contacts().Display(events[i].Caller))
			slot.Show()
		} else {
			slot.Hide()
		}
	}
}

func renderContacts(slots []*systray.MenuItem, app *core.App) {
	if len(slots) == 0 {
		return
	}
	labels := app.Contacts().SlotLabels(len(slots))
	if len(labels) == 0 {
		slots[0].SetTitle(noContactsLabel)
		slots[0].Show()
		for _, s := range slots[1:] {
			s.Hide()
		}
		return
	}
	// Ascending iteration only: Windows appends re-shown items in Show()
	// call order.
	for i, slot := range slots {
		if i < len(labels) {
			slot.SetTitle(labels[i])
			slot.Show()
		} else {
			slot.Hide()
		}
	}
}

func onReady(app *core.App, shutdown func()) {
	started.Store(true)

	toggle = systray.AddMenuItem("Disable notifications", "Enable or disable incoming-call notifications")
	showItem = systray.AddMenuItem("Show notifications", "Enable or disable the system notification")
	soundItem = systray.AddMenuItem("Play sound", "Enable or disable the call beep")

	refresh := func() {
		notifyOn := app.NotifyEnabled()
		beepOn := app.BeepEnabled()
		on := notifyOn || beepOn
		systray.SetIcon(trayIconBytes(on))
		if on {
			systray.SetTooltip("YealinkNotifier — notifications ON")
		} else {
			systray.SetTooltip("YealinkNotifier — notifications OFF")
		}
		if notifyOn && beepOn {
			toggle.SetTitle("Disable notifications")
			toggle.Check()
		} else {
			toggle.SetTitle("Enable notifications")
			toggle.Uncheck()
		}
		if notifyOn {
			showItem.Check()
		} else {
			showItem.Uncheck()
		}
		if beepOn {
			soundItem.Check()
		} else {
			soundItem.Uncheck()
		}
	}
	refresh()

	go func() {
		for range toggle.ClickedCh {
			on := !(app.NotifyEnabled() && app.BeepEnabled())
			app.SetNotify(on)
			app.SetBeep(on)
			if on {
				log.Println("Notifications enabled")
			} else {
				log.Println("Notifications disabled")
			}
			refresh()
		}
	}()
	go func() {
		for range showItem.ClickedCh {
			on := !app.NotifyEnabled()
			app.SetNotify(on)
			if on {
				log.Println("Show notifications ON")
			} else {
				log.Println("Show notifications OFF")
			}
			refresh()
		}
	}()
	go func() {
		for range soundItem.ClickedCh {
			on := !app.BeepEnabled()
			app.SetBeep(on)
			if on {
				log.Println("Sound ON")
			} else {
				log.Println("Sound OFF")
			}
			refresh()
		}
	}()

	systray.AddSeparator()

	contactsItem := systray.AddMenuItem("Contacts", "View the contacts book")
	contactSlots := make([]*systray.MenuItem, contactsMenuCapacity)
	for i := range contactSlots {
		contactSlots[i] = contactsItem.AddSubMenuItem("", "")
	}

	editItem := systray.AddMenuItem("Edit contacts", "Open the contacts CSV file in the default application")
	go func() {
		for range editItem.ClickedCh {
			book := app.Contacts()
			if err := book.EnsureFile(); err != nil {
				log.Printf("Error creating contacts file (%s): %v", book.Path(), err)
				continue
			}
			log.Printf("Opening contacts file: %s", book.Path())
			if err := book.Open(); err != nil {
				log.Printf("Error opening contacts file: %v", err)
			}
		}
	}()

	systray.AddSeparator()

	history := systray.AddMenuItem("History", "Incoming call history")
	histSlots := make([]*systray.MenuItem, core.HistoryCapacity)
	for i := range histSlots {
		histSlots[i] = history.AddSubMenuItem("", "")
	}

	renderCh := make(chan struct{}, 1)
	clearItem := systray.AddMenuItem("Clear history", "Clear the incoming call history")
	go func() {
		for range clearItem.ClickedCh {
			app.ClearHistory()
			log.Println("Call history cleared")
			select {
			case renderCh <- struct{}{}:
			default:
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		lastHist := app.HistoryVersion()
		lastBook := app.Contacts().Version()
		renderHistory(histSlots, app)
		renderContacts(contactSlots, app)
		for {
			select {
			case <-app.Done():
				return
			case <-ticker.C:
				// An idle tick stats once; a re-render adds one stat for the
				// contacts snapshot plus one per history row (Display → Lookup).
				bv := app.Contacts().Version()
				if v := app.HistoryVersion(); v != lastHist || bv != lastBook {
					lastHist = v
					lastBook = bv
					renderHistory(histSlots, app)
					renderContacts(contactSlots, app)
				}
			case <-renderCh:
				lastHist = app.HistoryVersion()
				lastBook = app.Contacts().Version()
				renderHistory(histSlots, app)
				renderContacts(contactSlots, app)
			}
		}
	}()

	systray.AddSeparator()

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
