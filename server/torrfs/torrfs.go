package torrfs

import "server/settings"

func New() *RootDir {
	return NewWithProvider(settings.DefaultSettingsProvider)
}

func NewWithProvider(provider settings.SettingsProvider) *RootDir {
	r := NewRootDir(provider)

	return r
}
