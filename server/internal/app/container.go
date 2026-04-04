package app

import (
	"server/config"
)

// Container keeps explicit runtime dependencies composed at startup.
type Container struct {
	Runtime Runtime
}

// NewContainer builds default application container.
func NewContainer() *Container {
	return NewContainerWithConfig(nil)
}

// NewContainerWithConfig builds container with optional config.
func NewContainerWithConfig(cfg *config.Config) *Container {
	return &Container{
		Runtime: newServerRuntime(defaultServerRuntimeDeps, cfg),
	}
}
