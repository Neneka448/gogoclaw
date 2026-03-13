package cron

import (
	"errors"
	"testing"
	"time"
)

type testCron struct {
	config Config
	runCh  chan struct{}
	err    error
}

func (cronTask *testCron) Execute() error {
	if cronTask.runCh != nil {
		select {
		case cronTask.runCh <- struct{}{}:
		default:
		}
	}
	return cronTask.err
}

func (cronTask *testCron) GetCronConfig() *Config {
	config := cronTask.config
	return &config
}

func TestCronManagerRegisterGetDelete(t *testing.T) {
	manager := NewCronManager(nil)
	cronTask := &testCron{config: Config{CronID: "nightly", CronExpression: "@every 1m", Enabled: true}}

	if err := manager.RegisterCron(cronTask); err != nil {
		t.Fatalf("RegisterCron() error = %v", err)
	}
	stored, err := manager.GetCron("nightly")
	if err != nil {
		t.Fatalf("GetCron() error = %v", err)
	}
	if stored.GetCronConfig().CronID != "nightly" {
		t.Fatalf("stored cron id = %q, want nightly", stored.GetCronConfig().CronID)
	}
	if err := manager.DeleteCron("nightly"); err != nil {
		t.Fatalf("DeleteCron() error = %v", err)
	}
	if _, err := manager.GetCron("nightly"); !errors.Is(err, ErrCronNotFound) {
		t.Fatalf("GetCron() error = %v, want ErrCronNotFound", err)
	}
}

func TestCronManagerStartStopAreIdempotent(t *testing.T) {
	manager := NewCronManager(nil)
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() first error = %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() second error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() first error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() second error = %v", err)
	}
}

func TestCronManagerStopClearsRuntimeCache(t *testing.T) {
	manager := NewCronManager(nil)
	cronTask := &testCron{config: Config{CronID: "nightly", CronExpression: "@every 1m", Enabled: true}}
	if err := manager.RegisterCron(cronTask); err != nil {
		t.Fatalf("RegisterCron() error = %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if _, err := manager.GetCron("nightly"); !errors.Is(err, ErrCronNotFound) {
		t.Fatalf("GetCron() error = %v, want ErrCronNotFound after stop", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() after stop error = %v", err)
	}
	if _, err := manager.GetCron("nightly"); !errors.Is(err, ErrCronNotFound) {
		t.Fatalf("GetCron() after restart error = %v, want ErrCronNotFound before reload", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() after restart error = %v", err)
	}
}

func TestCronManagerExecutesRegisteredCron(t *testing.T) {
	manager := NewCronManager(nil)
	runCh := make(chan struct{}, 1)
	cronTask := &testCron{config: Config{CronID: "fast", CronExpression: "@every 100ms", Enabled: true}, runCh: runCh}
	if err := manager.RegisterCron(cronTask); err != nil {
		t.Fatalf("RegisterCron() error = %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := manager.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}()

	select {
	case <-runCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cron execution")
	}
}
