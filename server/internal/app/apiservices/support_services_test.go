package apiservices

import (
	"reflect"
	"testing"
	"time"

	"server/internal/app/contracts"
	sets "server/settings"
	"server/torznab"

	goffprobe "gopkg.in/vansante/go-ffprobe.v2"
)

func TestMapTorznabResultsUsesApplicationSearchResultDTO(t *testing.T) {
	createdAt := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)

	got := mapTorznabResults([]*torznab.TorrentDetails{
		nil,
		{
			Title:      "Title",
			Name:       "Name",
			Link:       "https://example.test/torrent",
			Magnet:     "magnet:?xt=urn:btih:abc",
			Hash:       "abc",
			Size:       "42 GB",
			Seed:       10,
			Peer:       2,
			CreateDate: createdAt,
			Categories: []string{"movie", "uhd"},
			Year:       2026,
		},
	})

	want := []*contracts.SearchResult{
		{
			Title:      "Title",
			Name:       "Name",
			Link:       "https://example.test/torrent",
			Magnet:     "magnet:?xt=urn:btih:abc",
			Hash:       "abc",
			Size:       "42 GB",
			Seed:       10,
			Peer:       2,
			CreateDate: createdAt,
			Categories: []string{"movie", "uhd"},
			Year:       2026,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mapped results:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMapTorznabResultsReturnsNilForEmptyInput(t *testing.T) {
	if got := mapTorznabResults(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %#v", got)
	}

	if got := mapTorznabResults([]*torznab.TorrentDetails{}); got != nil {
		t.Fatalf("expected nil for empty input, got %#v", got)
	}
}

func TestMapViewedItemsUseApplicationDTO(t *testing.T) {
	got := mapViewedItemsFromSettings([]*sets.Viewed{
		nil,
		{Hash: "abc", FileIndex: 2},
	})

	want := []*contracts.ViewedItem{
		{Hash: "abc", FileIndex: 2},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mapped viewed items:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMapViewedItemToSettingsKeepsLegacyShape(t *testing.T) {
	got := mapViewedItemToSettings(&contracts.ViewedItem{Hash: "abc", FileIndex: 2})
	want := &sets.Viewed{Hash: "abc", FileIndex: 2}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected viewed settings item:\n got: %#v\nwant: %#v", got, want)
	}

	if got := mapViewedItemToSettings(nil); got != nil {
		t.Fatalf("expected nil settings item, got %#v", got)
	}
}

func TestMapMediaProbeUsesApplicationDTO(t *testing.T) {
	got, err := mapMediaProbe(&goffprobe.ProbeData{})
	if err != nil {
		t.Fatalf("mapMediaProbe returned error: %v", err)
	}

	if got == nil {
		t.Fatal("expected non-nil media probe DTO")
	}

	requireMediaProbeDTO(got)
}

func TestMapMediaProbeReturnsNilForNilInput(t *testing.T) {
	got, err := mapMediaProbe(nil)
	if err != nil {
		t.Fatalf("mapMediaProbe returned error: %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil media probe, got %#v", got)
	}
}

func requireMediaProbeDTO(contracts.MediaProbe) {}
