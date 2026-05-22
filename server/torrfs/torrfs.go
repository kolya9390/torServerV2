package torrfs

import "server/settings"

// New is a legacy compatibility constructor backed by the process-global settings provider.
// Production composition should call NewWithProvider.
func New() *RootDir {
	return NewWithProvider(settings.DefaultSettingsProvider)
}

func NewWithProvider(provider settings.SettingsProvider) *RootDir {
	r := NewRootDir(provider)

	return r
}
