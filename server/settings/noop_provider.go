package settings

type noopSettingsProvider struct{}

// NewNoopSettingsProvider returns a safe inert provider for non-runtime paths.
// It avoids silently binding callers to the process-global settings singleton.
func NewNoopSettingsProvider() SettingsProvider {
	return noopSettingsProvider{}
}

func (noopSettingsProvider) Get() *BTSets {
	return &BTSets{}
}

func (noopSettingsProvider) Set(*BTSets) {}

func (noopSettingsProvider) ReadOnly() bool {
	return true
}

func (noopSettingsProvider) GetStaticConfig() StaticConfig {
	return StaticConfig{}
}

func (noopSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}
