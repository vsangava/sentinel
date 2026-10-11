package main

import (
	"testing"
	"time"

	"github.com/vsangava/sentinel/internal/config"
)

func useTempConfig(t *testing.T) {
	t.Helper()
	oldDir := config.ConfigDirOverride
	oldLocal := config.UseLocalConfig
	config.ConfigDirOverride = t.TempDir()
	config.UseLocalConfig = false
	if err := config.LoadConfig(); err != nil {
		t.Fatalf("load temporary config: %v", err)
	}
	t.Cleanup(func() {
		config.ConfigDirOverride = oldDir
		config.UseLocalConfig = oldLocal
	})
}

func TestPauseSentinelPersistsPause(t *testing.T) {
	useTempConfig(t)
	now := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)

	until, err := pauseSentinel([]string{"30"}, now)
	if err != nil {
		t.Fatalf("pauseSentinel: %v", err)
	}
	if !until.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("pause until = %s, want %s", until, now.Add(30*time.Minute))
	}
	if err := config.LoadConfig(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if got := config.GetConfig().Pause.Until; !got.Equal(until) {
		t.Errorf("saved pause until = %s, want %s", got, until)
	}
}

func TestPauseSentinelValidatesDurationAndPomodoroLock(t *testing.T) {
	useTempConfig(t)
	now := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.UTC)
	for _, args := range [][]string{{}, {"0"}, {"241"}, {"abc"}, {"30", "extra"}} {
		if _, err := pauseSentinel(args, now); err == nil {
			t.Errorf("pauseSentinel(%v) expected an error", args)
		}
	}

	cfg := config.GetConfig()
	cfg.Pomodoro = &config.PomodoroSession{Phase: "work", PhaseEndsAt: now.Add(time.Hour)}
	config.AppConfig = cfg
	if err := config.SaveConfig(); err != nil {
		t.Fatalf("save Pomodoro fixture: %v", err)
	}
	if _, err := pauseSentinel([]string{"30"}, now); err == nil {
		t.Fatal("expected pause to be rejected during Pomodoro work")
	}
}

func TestResumeSentinelClearsPause(t *testing.T) {
	useTempConfig(t)
	config.SetPause(time.Now().Add(time.Hour))
	if err := config.SaveConfig(); err != nil {
		t.Fatalf("save initial pause: %v", err)
	}

	if err := resumeSentinel(nil); err != nil {
		t.Fatalf("resumeSentinel: %v", err)
	}
	if err := config.LoadConfig(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if config.GetConfig().Pause != nil {
		t.Errorf("expected pause to be cleared, got %+v", config.GetConfig().Pause)
	}
	if err := resumeSentinel([]string{"extra"}); err == nil {
		t.Fatal("expected resume arguments to be rejected")
	}
}
