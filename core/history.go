package core

import (
	"time"
)

const HistoryCapacity = 20

type Event struct {
	Time   time.Time
	Caller string
}

func (a *App) RecordEvent(caller string) {
	if caller == "" {
		caller = "unknown"
	}
	a.histMu.Lock()
	a.hist = append(a.hist, Event{Time: time.Now(), Caller: caller})
	if len(a.hist) > HistoryCapacity {
		a.hist = a.hist[len(a.hist)-HistoryCapacity:]
	}
	a.histVer.Add(1)
	a.histMu.Unlock()
}

func (a *App) Events() []Event {
	a.histMu.Lock()
	events := make([]Event, len(a.hist))
	copy(events, a.hist)
	a.histMu.Unlock()
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}

func (a *App) ClearHistory() {
	a.histMu.Lock()
	a.hist = a.hist[:0]
	a.histVer.Add(1)
	a.histMu.Unlock()
}

func (a *App) HistoryVersion() uint64 {
	return a.histVer.Load()
}
