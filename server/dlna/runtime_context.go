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

func (runtimeCtx dlnaRuntimeContext) currentSettings() *settings.BTSets {
	return runtimeCtx.settingsProvider.Get()
}

func (runtimeCtx dlnaRuntimeContext) currentArgs() *settings.ExecArgs {
	return runtimeCtx.argsProvider.Get()
}
