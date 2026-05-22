package apiservices

import (
	"reflect"
	"testing"
	"time"

	"server/internal/app/contracts"
	"server/torznab"
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
