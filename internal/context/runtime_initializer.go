package context

type RuntimeInitializer struct {
	context SystemContext
}

func NewRuntimeInitializer(systemContext SystemContext) RuntimeInitializer {
	return RuntimeInitializer{context: systemContext}
}

func (initializer RuntimeInitializer) EnsureReady() error {
	if initializer.context.VectorStore != nil {
		if err := initializer.context.VectorStore.Start(); err != nil {
			return err
		}
	}

	if initializer.context.MemoryService != nil && initializer.context.MemoryEnabled {
		if err := initializer.context.MemoryService.Initialize(); err != nil {
			if initializer.context.VectorStore != nil {
				_ = initializer.context.VectorStore.Stop()
			}
			return err
		}
	}

	return nil
}
