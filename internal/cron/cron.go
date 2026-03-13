package cron

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	ccron "github.com/robfig/cron/v3"
)

var ErrCronNotFound = errors.New("cron not found")

type Config struct {
	CronExpression string `json:"cronExpression"`
	CronID         string `json:"cronID"`
	Enabled        bool   `json:"enabled"`
}

type Cron interface {
	Execute() error
	GetCronConfig() *Config
}

type CronManager interface {
	RegisterCron(cron Cron) error
	GetCron(cronID string) (Cron, error)
	DeleteCron(cronID string) error

	Start() error
	Stop() error
}

type cronManger struct {
	mu       sync.Mutex
	cache    map[string]Cron
	entries  map[string]ccron.EntryID
	location *time.Location
	holder   *ccron.Cron
	started  bool
}

func NewCronManager(location *time.Location) CronManager {
	if location == nil {
		location = time.Local
	}
	return &cronManger{
		cache:    make(map[string]Cron),
		entries:  make(map[string]ccron.EntryID),
		location: location,
		holder:   ccron.New(ccron.WithLocation(location)),
	}
}

func (c *cronManger) RegisterCron(cron Cron) error {
	if cron == nil {
		return fmt.Errorf("cron is nil")
	}
	config := cron.GetCronConfig()
	if config == nil {
		return fmt.Errorf("cron config is nil")
	}
	cronID := strings.TrimSpace(config.CronID)
	if cronID == "" {
		return fmt.Errorf("cron id is required")
	}
	cronExpression := strings.TrimSpace(config.CronExpression)
	if cronExpression == "" {
		return fmt.Errorf("cron expression is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existingEntryID, ok := c.entries[cronID]; ok {
		c.holder.Remove(existingEntryID)
		delete(c.entries, cronID)
	}

	entryID, err := c.holder.AddFunc(cronExpression, func() {
		_ = cron.Execute()
	})
	if err != nil {
		return err
	}

	c.cache[cronID] = cron
	c.entries[cronID] = entryID
	return nil
}

func (c *cronManger) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}
	c.holder.Start()
	c.started = true
	return nil
}

func (c *cronManger) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		stopContext := c.holder.Stop()
		<-stopContext.Done()
	}

	c.holder = ccron.New(ccron.WithLocation(c.location))
	c.entries = make(map[string]ccron.EntryID)
	c.cache = make(map[string]Cron)
	c.started = false
	return nil
}

func (c *cronManger) DeleteCron(cronID string) error {
	cronID = strings.TrimSpace(cronID)
	if cronID == "" {
		return fmt.Errorf("cron id is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[cronID]; !ok {
		return fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
	}
	if entryID, ok := c.entries[cronID]; ok {
		c.holder.Remove(entryID)
		delete(c.entries, cronID)
	}
	delete(c.cache, cronID)
	return nil
}

func (c *cronManger) GetCron(cronID string) (Cron, error) {
	cronID = strings.TrimSpace(cronID)
	if cronID == "" {
		return nil, fmt.Errorf("cron id is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	scheduledCron, ok := c.cache[cronID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCronNotFound, cronID)
	}
	return scheduledCron, nil
}
