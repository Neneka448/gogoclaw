package cron

import (
	"github.com/Neneka448/gogoclaw/internal/context"
	ccron "github.com/robfig/cron/v3"
)

type Config struct {
	CronExpression string `json:"cronExpression"`
	Prompt         string `json:"prompt"`
	CronID         string `json:"cronID"`
}

type Cron interface {
	Execute() error
	GetCronConfig() *Config
}

type CronManager interface {
	RegisterCron(cron Cron)
	GetCron(cronID string) (Cron, error)
	DeleteCron(cronID string) error

	Start()
	Stop()
}

type cronManger struct {
	cache  map[string]Cron
	holder *ccron.Cron
}

func NewCronManager(context context.SystemContext) CronManager {
	return &cronManger{
		cache:  make(map[string]Cron),
		holder: ccron.New(),
	}
}

func (c *cronManger) RegisterCron(cron Cron) {}

func (c *cronManger) Start() {

}

func (c *cronManger) Stop() {}

func (c *cronManger) DeleteCron(cronID string) error {}

func (c *cronManger) GetCron(cronID string) (Cron, error) {}
