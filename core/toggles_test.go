package core

import "testing"

func TestNewInitialState(t *testing.T) {
	tests := []struct {
		name   string
		notify bool
		beep   bool
	}{
		{name: "both on", notify: true, beep: true},
		{name: "notification only", notify: true, beep: false},
		{name: "beep only", notify: false, beep: true},
		{name: "both off", notify: false, beep: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.notify, tt.beep)
			if a.NotifyEnabled() != tt.notify {
				t.Errorf("NotifyEnabled() = %v, want %v", a.NotifyEnabled(), tt.notify)
			}
			if a.BeepEnabled() != tt.beep {
				t.Errorf("BeepEnabled() = %v, want %v", a.BeepEnabled(), tt.beep)
			}
		})
	}
}

func TestSetNotifySetBeep(t *testing.T) {
	tests := []struct {
		name       string
		notify0    bool
		beep0      bool
		do         func(a *App)
		wantNotify bool
		wantBeep   bool
	}{
		{
			name:       "SetNotify off leaves beep untouched",
			notify0:    true,
			beep0:      true,
			do:         func(a *App) { a.SetNotify(false) },
			wantNotify: false,
			wantBeep:   true,
		},
		{
			name:       "SetBeep on leaves notify untouched",
			notify0:    true,
			beep0:      false,
			do:         func(a *App) { a.SetBeep(true) },
			wantNotify: true,
			wantBeep:   true,
		},
		{
			name:       "SetBeep off leaves notify untouched",
			notify0:    true,
			beep0:      true,
			do:         func(a *App) { a.SetBeep(false) },
			wantNotify: true,
			wantBeep:   false,
		},
		{
			name:       "SetNotify on from off",
			notify0:    false,
			beep0:      false,
			do:         func(a *App) { a.SetNotify(true) },
			wantNotify: true,
			wantBeep:   false,
		},
		{
			name:       "both off from both on",
			notify0:    true,
			beep0:      true,
			do:         func(a *App) { a.SetNotify(false); a.SetBeep(false) },
			wantNotify: false,
			wantBeep:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.notify0, tt.beep0)
			tt.do(a)
			if got := a.NotifyEnabled(); got != tt.wantNotify {
				t.Errorf("NotifyEnabled() = %v, want %v", got, tt.wantNotify)
			}
			if got := a.BeepEnabled(); got != tt.wantBeep {
				t.Errorf("BeepEnabled() = %v, want %v", got, tt.wantBeep)
			}
		})
	}
}

func TestAnyEnabled(t *testing.T) {
	tests := []struct {
		name   string
		notify bool
		beep   bool
		want   bool
	}{
		{name: "both on", notify: true, beep: true, want: true},
		{name: "notification only", notify: true, beep: false, want: true},
		{name: "beep only", notify: false, beep: true, want: true},
		{name: "both off", notify: false, beep: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.notify, tt.beep)
			if got := a.AnyEnabled(); got != tt.want {
				t.Errorf("AnyEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOnCallRecordsWithoutSideEffects(t *testing.T) {
	a := New(false, false) // both channels off: OnCall must only record, spawn nothing

	if a.AnyEnabled() {
		t.Fatal("fixture App unexpectedly has an enabled channel")
	}
	a.OnCall("144")

	events := a.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if events[0].Caller != "144" {
		t.Errorf("Events()[0].Caller = %q, want %q", events[0].Caller, "144")
	}

	// OnCall sanitizes the caller BEFORE recording it.
	a.OnCall("sip:100@10.184.5.37_number")
	ev := a.Events()[0]
	if ev.Caller != "100" {
		t.Errorf("Events()[0].Caller = %q, want %q", ev.Caller, "100")
	}

	// Empty caller is recorded as "unknown".
	a.OnCall("")
	ev = a.Events()[0]
	if ev.Caller != "unknown" {
		t.Errorf("Events()[0].Caller = %q, want %q", ev.Caller, "unknown")
	}
}
