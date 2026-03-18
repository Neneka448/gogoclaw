package context

import (
	"time"

	"github.com/Neneka448/gogoclaw/internal/utils"
)

type RuntimeInitializer struct {
	context SystemContext
}

func NewRuntimeInitializer(systemContext SystemContext) RuntimeInitializer {
	return RuntimeInitializer{context: systemContext}
}

func (initializer RuntimeInitializer) EnsureReady() error {
	if initializer.context.VectorStore != nil {
		t0 := time.Now()
		if err := initializer.context.VectorStore.Start(); err != nil {
			return err
		}
		utils.Perf("ensureReady: vectorstore start took %s", time.Since(t0))
	}

	if initializer.context.MemoryService != nil && initializer.context.MemoryEnabled {
		t0 := time.Now()
		if err := initializer.context.MemoryService.Initialize(); err != nil {
			if initializer.context.VectorStore != nil {
				_ = initializer.context.VectorStore.Stop()
			}
			return err
		}
		utils.Perf("ensureReady: memory initialize took %s", time.Since(t0))
	}

	return nil
}
