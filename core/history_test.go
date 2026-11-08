package core

import (
	"fmt"
	"sync"
	"testing"
)

func TestHistoryAppendOrder(t *testing.T) {
	a := New(false, false)
	a.RecordEvent("111")
	a.RecordEvent("222")
	a.RecordEvent("333")

	events := a.Events()
	if len(events) != 3 {
		t.Fatalf("Events() len = %d, want 3", len(events))
	}
	want := []string{"333", "222", "111"} // newest first
	for i, ev := range events {
		if ev.Caller != want[i] {
			t.Errorf("Events()[%d].Caller = %q, want %q", i, ev.Caller, want[i])
		}
		if ev.Time.IsZero() {
			t.Errorf("Events()[%d].Time is zero, want a timestamp", i)
		}
	}
}

func TestHistoryCapacityOverflow(t *testing.T) {
	a := New(false, false)
	const total = HistoryCapacity + 5 // record 25, capacity keeps exactly 20
	for i := range total {
		a.RecordEvent(fmt.Sprintf("%03d", i))
	}

	events := a.Events()
	if len(events) != HistoryCapacity {
		t.Fatalf("Events() len = %d, want %d", len(events), HistoryCapacity)
	}
	// The 5 oldest (000..004) were dropped; the rest are kept newest first.
	for i, ev := range events {
		want := fmt.Sprintf("%03d", total-1-i)
		if ev.Caller != want {
			t.Errorf("Events()[%d].Caller = %q, want %q", i, ev.Caller, want)
		}
	}
}

func TestHistoryEmptyCallerBecomesUnknown(t *testing.T) {
	a := New(false, false)
	a.RecordEvent("")

	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if events[0].Caller != "unknown" {
		t.Errorf("Events()[0].Caller = %q, want %q", events[0].Caller, "unknown")
	}
}

func TestEventsDefensiveCopy(t *testing.T) {
	a := New(false, false)
	a.RecordEvent("111")

	events := a.Events()
	events[0].Caller = "tampered"
	events = append(events, Event{Caller: "injected"})

	stored := a.Events()
	if len(stored) != 1 {
		t.Fatalf("Events() len = %d after mutating the returned slice, want 1", len(stored))
	}
	if stored[0].Caller != "111" {
		t.Errorf("Events()[0].Caller = %q after mutating the returned slice, want %q", stored[0].Caller, "111")
	}
}

func TestClearHistoryEmptiesAndBumpsVersion(t *testing.T) {
	a := New(false, false)
	a.RecordEvent("111")
	a.RecordEvent("222")
	afterRecords := a.HistoryVersion()

	a.ClearHistory()

	if got := a.Events(); len(got) != 0 {
		t.Errorf("Events() after ClearHistory = %v, want empty", got)
	}
	if got := a.HistoryVersion(); got != afterRecords+1 {
		t.Errorf("HistoryVersion() after ClearHistory = %d, want %d", got, afterRecords+1)
	}
}

func TestHistoryVersionIncrementsAndIsStableOnReads(t *testing.T) {
	a := New(false, false)
	steps := []struct {
		name string
		op   func()
		want uint64
	}{
		{name: "fresh app", op: func() {}, want: 0},
		{name: "first record", op: func() { a.RecordEvent("1") }, want: 1},
		{name: "second record", op: func() { a.RecordEvent("2") }, want: 2},
		{name: "clear", op: func() { a.ClearHistory() }, want: 3},
		{name: "record after clear", op: func() { a.RecordEvent("3") }, want: 4},
	}
	for _, s := range steps {
		s.op()
		if got := a.HistoryVersion(); got != s.want {
			t.Errorf("%s: HistoryVersion() = %d, want %d", s.name, got, s.want)
		}
	}
	// Reading events must not bump the version.
	if got := len(a.Events()); got != 1 {
		t.Fatalf("Events() len = %d, want 1", got)
	}
	if got := a.HistoryVersion(); got != steps[len(steps)-1].want {
		t.Errorf("HistoryVersion() after Events() read = %d, want %d", got, steps[len(steps)-1].want)
	}
}

func TestHistoryRecordConcurrent(t *testing.T) {
	a := New(false, false)
	const workers = 8
	const perWorker = 16

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				a.RecordEvent("111")
			}
		})
	}
	wg.Wait()

	if got := len(a.Events()); got != HistoryCapacity {
		t.Errorf("Events() len = %d, want %d (capacity cap)", got, HistoryCapacity)
	}
	if got := a.HistoryVersion(); got != workers*perWorker {
		t.Errorf("HistoryVersion() = %d, want %d", got, workers*perWorker)
	}
}
