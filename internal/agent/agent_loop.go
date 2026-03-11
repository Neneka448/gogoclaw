package agent

import "github.com/Neneka448/gogoclaw/internal/context"

type AgentLoop interface {
	processMessage(message string) error
}

type agentLoop struct {
	context context.SystemContext
}

func NewAgentLoop(context context.SystemContext) AgentLoop {
	return &agentLoop{
		context: context,
	}
}

func (al *agentLoop) processMessage(message string) error {
	return nil
}
