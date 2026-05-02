package config

import (
	"testing"
	"time"
)

func TestIsLockedByPomodoro_WorkPhaseInFuture(t *testing.T) {
	cfg := Config{
		Pomodoro: &PomodoroSession{
			Phase:       "work",
			PhaseEndsAt: time.Now().Add(10 * time.Minute),
		},
	}
	if !cfg.IsLockedByPomodoro(time.Now()) {
		t.Error("expected locked during active work phase")
	}
}

func TestIsLockedByPomodoro_WorkPhaseExpired(t *testing.T) {
	cfg := Config{
		Pomodoro: &PomodoroSession{
			Phase:       "work",
			PhaseEndsAt: time.Now().Add(-1 * time.Second),
		},
	}
	if cfg.IsLockedByPomodoro(time.Now()) {
		t.Error("expected not locked when work phase has expired")
	}
}

func TestIsLockedByPomodoro_BreakPhase(t *testing.T) {
	cfg := Config{
		Pomodoro: &PomodoroSession{
			Phase:       "break",
			PhaseEndsAt: time.Now().Add(5 * time.Minute),
		},
	}
	if cfg.IsLockedByPomodoro(time.Now()) {
		t.Error("expected not locked during break phase")
	}
}

func TestIsLockedByPomodoro_NilPomodoro(t *testing.T) {
	cfg := Config{}
	if cfg.IsLockedByPomodoro(time.Now()) {
		t.Error("expected not locked when no session exists")
	}
}

func TestStartPomodoro_SetsCorrectPhase(t *testing.T) {
	t.Cleanup(func() { AppConfig.Pomodoro = nil })

	before := time.Now()
	StartPomodoro(25, 5)
	after := time.Now()

	p := AppConfig.Pomodoro
	if p == nil {
		t.Fatal("expected Pomodoro to be set")
	}
	if p.Phase != "work" {
		t.Errorf("expected phase=work, got %q", p.Phase)
	}
	if p.WorkMinutes != 25 {
		t.Errorf("expected work_minutes=25, got %d", p.WorkMinutes)
	}
	if p.BreakMinutes != 5 {
		t.Errorf("expected break_minutes=5, got %d", p.BreakMinutes)
	}
	expectedMin := before.Add(24 * time.Minute)
	expectedMax := after.Add(26 * time.Minute)
	if p.PhaseEndsAt.Before(expectedMin) || p.PhaseEndsAt.After(expectedMax) {
		t.Errorf("PhaseEndsAt %v outside expected range [%v, %v]", p.PhaseEndsAt, expectedMin, expectedMax)
	}
}

func TestAdvancePomodoroPhase_WorkToBreak(t *testing.T) {
	t.Cleanup(func() { AppConfig.Pomodoro = nil })

	AppConfig.Pomodoro = &PomodoroSession{
		Phase:        "work",
		PhaseEndsAt:  time.Now().Add(-1 * time.Second),
		WorkMinutes:  25,
		BreakMinutes: 5,
	}

	before := time.Now()
	AdvancePomodoroPhase()
	after := time.Now()

	p := AppConfig.Pomodoro
	if p.Phase != "break" {
		t.Errorf("expected phase=break, got %q", p.Phase)
	}
	expectedMin := before.Add(4 * time.Minute)
	expectedMax := after.Add(6 * time.Minute)
	if p.PhaseEndsAt.Before(expectedMin) || p.PhaseEndsAt.After(expectedMax) {
		t.Errorf("break PhaseEndsAt %v outside expected range", p.PhaseEndsAt)
	}
}

func TestAdvancePomodoroPhase_NoopWhenNil(t *testing.T) {
	t.Cleanup(func() { AppConfig.Pomodoro = nil })
	AppConfig.Pomodoro = nil
	AdvancePomodoroPhase() // must not panic
}

func TestClearPomodoro(t *testing.T) {
	AppConfig.Pomodoro = &PomodoroSession{Phase: "work"}
	ClearPomodoro()
	if AppConfig.Pomodoro != nil {
		t.Error("expected Pomodoro to be nil after ClearPomodoro")
	}
}
