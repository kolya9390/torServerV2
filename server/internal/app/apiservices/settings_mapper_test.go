package apiservices

import (
	"reflect"
	"testing"

	"server/internal/app/contracts"
	sets "server/settings"
)

func TestMapSettingsFromBTSetsUsesApplicationDTO(t *testing.T) {
	src := &sets.BTSets{
		CacheSize:           1024,
		EnableDLNA:          true,
		EnableTorznabSearch: true,
		TorznabUrls: []sets.TorznabConfig{
			{Host: "https://torznab.test", Key: "key", Name: "main"},
		},
		TMDBSettings: sets.TMDBConfig{
			APIKey:     "tmdb-key",
			APIURL:     "https://api.test",
			ImageURL:   "https://img.test",
			ImageURLRu: "https://img-ru.test",
		},
		ProxyHosts: []string{"*.example.test"},
	}

	got := mapSettingsFromBTSets(src)
	want := &contracts.Settings{
		CacheSize:           1024,
		EnableDLNA:          true,
		EnableTorznabSearch: true,
		TorznabUrls: []contracts.TorznabConfig{
			{Host: "https://torznab.test", Key: "key", Name: "main"},
		},
		TMDBSettings: contracts.TMDBConfig{
			APIKey:     "tmdb-key",
			APIURL:     "https://api.test",
			ImageURL:   "https://img.test",
			ImageURLRu: "https://img-ru.test",
		},
		ProxyHosts: []string{"*.example.test"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mapped settings:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMapSettingsToBTSetsKeepsLegacyShape(t *testing.T) {
	src := &contracts.Settings{
		CacheSize:           2048,
		EnableDLNA:          true,
		EnableTorznabSearch: true,
		TorznabUrls: []contracts.TorznabConfig{
			{Host: "https://torznab.test", Key: "key", Name: "main"},
		},
		TMDBSettings: contracts.TMDBConfig{
			APIKey:     "tmdb-key",
			APIURL:     "https://api.test",
			ImageURL:   "https://img.test",
			ImageURLRu: "https://img-ru.test",
		},
		ProxyHosts: []string{"*.example.test"},
	}

	got := mapSettingsToBTSets(src)
	want := &sets.BTSets{
		CacheSize:           2048,
		EnableDLNA:          true,
		EnableTorznabSearch: true,
		TorznabUrls: []sets.TorznabConfig{
			{Host: "https://torznab.test", Key: "key", Name: "main"},
		},
		TMDBSettings: sets.TMDBConfig{
			APIKey:     "tmdb-key",
			APIURL:     "https://api.test",
			ImageURL:   "https://img.test",
			ImageURLRu: "https://img-ru.test",
		},
		ProxyHosts: []string{"*.example.test"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mapped BTSets:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSettingsMappersReturnNilForNilInput(t *testing.T) {
	if got := mapSettingsFromBTSets(nil); got != nil {
		t.Fatalf("expected nil settings DTO, got %#v", got)
	}

	if got := mapSettingsToBTSets(nil); got != nil {
		t.Fatalf("expected nil BTSets, got %#v", got)
	}
}
