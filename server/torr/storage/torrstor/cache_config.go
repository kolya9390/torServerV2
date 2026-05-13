package torrstor

import "server/settings"

func (c *Cache) currentCacheConfig() settings.CacheConfig {
	return c.currentSettings().CacheConfig()
}

func (c *Cache) currentPlaybackConfig() settings.PlaybackConfig {
	return c.currentSettings().PlaybackConfig()
}

func (c *Cache) currentNetworkConfig() settings.NetworkConfig {
	return c.currentSettings().NetworkConfig()
}
