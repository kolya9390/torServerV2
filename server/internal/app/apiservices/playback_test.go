package apiservices

import (
	"testing"

	sets "server/settings"
	"server/torr/state"
)

type mockViewedService struct {
	viewed []*mockViewed
}

type mockViewed struct {
	hash      string
	fileIndex int
}

func (m *mockViewedService) SetViewed(v *sets.Viewed)    {}
func (m *mockViewedService) RemoveViewed(v *sets.Viewed) {}

func (m *mockViewedService) ListViewed(hash string) []*sets.Viewed {
	var result []*sets.Viewed

	for _, v := range m.viewed {
		if v.hash == hash {
			result = append(result, &sets.Viewed{Hash: v.hash, FileIndex: v.fileIndex})
		}
	}

	return result
}

func TestNormalizePlaylistName(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{"empty raw", "", "movie", "movie.m3u"},
		{"with slash prefix", "/movie", "fallback", "movie.m3u"},
		{"with m3u extension", "movie.m3u", "fallback", "movie.m3u"},
		{"with m3u8 extension", "movie.m3u8", "fallback", "movie.m3u8"},
		{"plain name", "movie", "fallback", "movie.m3u"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePlaylistName(tt.raw, tt.fallback)
			if got != tt.want {
				t.Errorf("normalizePlaylistName(%q, %q) = %q, want %q", tt.raw, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestFindFileNamesakes(t *testing.T) {
	files := []*state.TorrentFileStat{
		{Id: 1, Path: "movie.avi"},
		{Id: 2, Path: "movie.avi.srt"},
		{Id: 3, Path: "movie.avi.eng.srt"},
		{Id: 4, Path: "other.avi"},
	}

	result := findFileNamesakes(files, files[0])

	if len(result) != 2 {
		t.Errorf("expected 2 namesakes, got %d", len(result))
	}

	if result[0].Id != 2 || result[1].Id != 3 {
		t.Errorf("unexpected namesakes: %+v", result)
	}
}

func TestFindFileNamesakes_NoMatches(t *testing.T) {
	files := []*state.TorrentFileStat{
		{Id: 1, Path: "video.mp4"},
		{Id: 2, Path: "audio.mp3"},
	}

	result := findFileNamesakes(files, files[0])

	if len(result) != 0 {
		t.Errorf("expected 0 namesakes, got %d", len(result))
	}
}

func TestSearchLastPlayed_NoViewed(t *testing.T) {
	viewedSvc := &mockViewedService{viewed: []*mockViewed{}}
	tor := &state.TorrentStatus{
		Hash: "abc123",
		FileStats: []*state.TorrentFileStat{
			{Id: 1, Path: "video.mp4"},
		},
	}

	result := searchLastPlayed(viewedSvc, tor)

	if result != -1 {
		t.Errorf("expected -1 for no viewed, got %d", result)
	}
}

func TestSearchLastPlayed_Found(t *testing.T) {
	viewedSvc := &mockViewedService{
		viewed: []*mockViewed{
			{hash: "abc123", fileIndex: 2},
			{hash: "abc123", fileIndex: 1},
		},
	}
	tor := &state.TorrentStatus{
		Hash: "abc123",
		FileStats: []*state.TorrentFileStat{
			{Id: 1, Path: "video1.mp4"},
			{Id: 2, Path: "video2.mp4"},
		},
	}

	result := searchLastPlayed(viewedSvc, tor)

	if result != 1 {
		t.Errorf("expected 1 (index of FileIndex 2), got %d", result)
	}
}

func TestSearchLastPlayed_IndexOutOfBounds(t *testing.T) {
	viewedSvc := &mockViewedService{
		viewed: []*mockViewed{
			{hash: "abc123", fileIndex: 99},
		},
	}
	tor := &state.TorrentStatus{
		Hash: "abc123",
		FileStats: []*state.TorrentFileStat{
			{Id: 1, Path: "video1.mp4"},
		},
	}

	result := searchLastPlayed(viewedSvc, tor)

	if result != -1 {
		t.Errorf("expected -1 for out of bounds index, got %d", result)
	}
}
