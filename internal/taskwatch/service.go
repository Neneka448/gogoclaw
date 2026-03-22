package taskwatch

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Neneka448/gogoclaw/internal/cron"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

const (
	DefaultCheckInterval = 60 * time.Second
	DefaultTimeout       = 3600 * time.Second
	scanInterval         = 30 * time.Second
)

// Service watches invocation directories for completion and injects
// notification messages into the gateway inbound queue.
type Service interface {
	Start() error
	Stop() error
	Register(entry WatchEntry) error
	Unregister(invocationID string) error
	List() ([]WatchEntry, error)
}

// Options configures a new taskwatch service.
type Options struct {
	Workspace   string
	MessageBus  messagebus.MessageBus
	CronService cron.Service
}

type service struct {
	store       *Store
	messageBus  messagebus.MessageBus
	cronService cron.Service
	stopCh      chan struct{}
	wg          sync.WaitGroup
	mu          sync.Mutex
	started     bool
}

func NewService(opts Options) Service {
	return &service{
		store:       NewStore(opts.Workspace),
		messageBus:  opts.MessageBus,
		cronService: opts.CronService,
	}
}

func (s *service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	s.stopCh = make(chan struct{})
	s.started = true
	s.wg.Add(1)
	go s.scanLoop()
	return nil
}

func (s *service) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	close(s.stopCh)
	s.started = false
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *service) Register(entry WatchEntry) error {
	if entry.InvocationID == "" {
		return fmt.Errorf("invocation_id is required")
	}
	if entry.InvocationDir == "" {
		return fmt.Errorf("invocation_dir is required")
	}
	if entry.CallerProfile == "" {
		return fmt.Errorf("caller_profile is required")
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.LastCheckedAt.IsZero() {
		entry.LastCheckedAt = now
	}
	if time.Duration(entry.CheckInterval) <= 0 {
		entry.CheckInterval = Duration(DefaultCheckInterval)
	}
	if time.Duration(entry.Timeout) <= 0 {
		entry.Timeout = Duration(DefaultTimeout)
	}
	entry.Status = WatchStatusActive
	return s.store.Put(entry)
}

func (s *service) Unregister(invocationID string) error {
	entry, err := s.store.Get(invocationID)
	if err != nil {
		return err
	}
	if entry == nil {
		return nil
	}
	entry.Status = WatchStatusCompleted
	return s.store.Put(*entry)
}

func (s *service) List() ([]WatchEntry, error) {
	return s.store.ListAll()
}

func (s *service) scanLoop() {
	defer s.wg.Done()

	// Run an initial scan immediately on start.
	s.scan()

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.scan()
		}
	}
}

func (s *service) scan() {
	entries, err := s.store.ListActive()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] list active watches: %v\n", err)
		return
	}
	now := time.Now().UTC()
	for _, entry := range entries {
		s.checkEntry(entry, now)
	}
}

func (s *service) checkEntry(entry WatchEntry, now time.Time) {
	// Respect per-entry check interval.
	nextCheck := entry.LastCheckedAt.Add(time.Duration(entry.CheckInterval))
	if now.Before(nextCheck) {
		return
	}

	status, err := readInvocationStatus(entry.InvocationDir)
	if err != nil {
		// Directory might not exist yet if task hasn't started.
		entry.LastCheckedAt = now
		_ = s.store.Put(entry)
		return
	}

	switch status.Status {
	case "succeeded", "failed":
		s.handleCompletion(entry, status, now)
	default:
		// Still pending or running — check timeout.
		if now.After(entry.CreatedAt.Add(time.Duration(entry.Timeout))) {
			s.handleTimeout(entry, now)
		} else {
			entry.LastCheckedAt = now
			_ = s.store.Put(entry)
		}
	}
}

func (s *service) handleCompletion(entry WatchEntry, status *InvocationStatus, now time.Time) {
	msg := buildCompletionMessage(entry, status, now)
	if err := s.messageBus.Put(msg, messagebus.InboundQueue); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] inject completion %s: %v\n", entry.InvocationID, err)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] task %s %s — notified caller %s\n", entry.InvocationID, status.Status, entry.CallerProfile)
	s.disableTaskCron(entry)
	entry.Status = WatchStatusCompleted
	entry.LastCheckedAt = now
	_ = s.store.Put(entry)
}

func (s *service) handleTimeout(entry WatchEntry, now time.Time) {
	msg := buildTimeoutMessage(entry, now)
	if err := s.messageBus.Put(msg, messagebus.InboundQueue); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] inject timeout %s: %v\n", entry.InvocationID, err)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] task %s timed out after %s\n", entry.InvocationID, time.Duration(entry.Timeout))
	s.disableTaskCron(entry)
	entry.Status = WatchStatusTimeout
	entry.LastCheckedAt = now
	_ = s.store.Put(entry)
}

func (s *service) disableTaskCron(entry WatchEntry) {
	if s.cronService == nil || entry.TaskCronID == "" {
		return
	}
	existing, err := s.cronService.GetCron(entry.TaskCronID)
	if err != nil || existing == nil {
		return
	}
	if !existing.Config.Enabled {
		return
	}
	_, err = s.cronService.UpdateCron(cron.UpsertCronInput{
		CronID:         existing.Config.CronID,
		CronExpression: existing.Config.CronExpression,
		Enabled:        false,
		Task:           existing.Task,
		ProfileName:    existing.Config.ProfileName,
		InvocationMode: existing.Config.InvocationMode,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[taskwatch] disable cron %s: %v\n", entry.TaskCronID, err)
	}
}
