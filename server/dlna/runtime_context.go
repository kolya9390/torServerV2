package dlna

import "server/settings"

type dlnaRuntimeContext struct {
	settingsProvider settings.SettingsProvider
	argsProvider     settings.ArgsProvider
}

func newDLNARuntimeContext(provider settings.SettingsProvider, argsProvider settings.ArgsProvider) dlnaRuntimeContext {
	if provider == nil {
		provider = settings.NewNoopSettingsProvider()
	}

	if argsProvider == nil {
		argsProvider = settings.NewNoopArgsProvider()
	}

	return dlnaRuntimeContext{
		settingsProvider: provider,
		argsProvider:     argsProvider,
	}
}

func (c dlnaRuntimeContext) currentSettings() *settings.BTSets {
	return c.settingsProvider.Get()
}

func (c dlnaRuntimeContext) currentArgs() *settings.ExecArgs {
	return c.argsProvider.Get()
}
