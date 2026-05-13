package fuse

import "server/settings"

type fuseRuntimeContext struct {
	settingsProvider settings.SettingsProvider
	argsProvider     settings.ArgsProvider
}

func newFuseRuntimeContext(provider settings.SettingsProvider, argsProvider settings.ArgsProvider) fuseRuntimeContext {
	if provider == nil {
		provider = settings.NewNoopSettingsProvider()
	}

	if argsProvider == nil {
		argsProvider = settings.NewNoopArgsProvider()
	}

	return fuseRuntimeContext{
		settingsProvider: provider,
		argsProvider:     argsProvider,
	}
}

func (c fuseRuntimeContext) currentSettings() *settings.BTSets {
	return c.settingsProvider.Get()
}

func (c fuseRuntimeContext) currentArgs() *settings.ExecArgs {
	return c.argsProvider.Get()
}
