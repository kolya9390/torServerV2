package api

import (
	"testing"

	sets "server/settings"
	"server/torr/state"
)

type testViewedSvc struct {
	viewed []*sets.Viewed
}

func (s *testViewedSvc) SetViewed(v *sets.Viewed)    {}
func (s *testViewedSvc) RemoveViewed(v *sets.Viewed) {}
func (s *testViewedSvc) ListViewed(hash string) []*sets.Viewed {
	return s.viewed
}

func TestFindFileNamesakes(t *testing.T) {
	files := []*state.TorrentFileStat{
		{ID: 1, Path: "movie.avi"},
		{ID: 2, Path: "movie.avi.srt"},
		{ID: 3, Path: "movie.avi.eng.srt"},
		{ID: 4, Path: "other.avi"},
	}

	result := findFileNamesakes(files, files[0])

	if len(result) != 2 {
		t.Errorf("expected 2 namesakes, got %d", len(result))
	}
}

func TestFindFileNamesakes_NoMatches(t *testing.T) {
	files := []*state.TorrentFileStat{
		{ID: 1, Path: "video.mp4"},
		{ID: 2, Path: "audio.mp3"},
	}

	result := findFileNamesakes(files, files[0])

	if len(result) != 0 {
		t.Errorf("expected 0 namesakes, got %d", len(result))
	}
}

func TestSearchLastPlayed_NoViewed(t *testing.T) {
	viewedSvc := &testViewedSvc{viewed: []*sets.Viewed{}}
	tor := &state.TorrentStatus{
		Hash: "abc123",
		FileStats: []*state.TorrentFileStat{
			{ID: 1, Path: "video.mp4"},
		},
	}

	result := searchLastPlayed(viewedSvc, tor)

	if result != -1 {
		t.Errorf("expected -1 for no viewed, got %d", result)
	}
}

func TestSearchLastPlayed_Found(t *testing.T) {
	viewedSvc := &testViewedSvc{
		viewed: []*sets.Viewed{
			{Hash: "abc123", FileIndex: 2},
			{Hash: "abc123", FileIndex: 1},
		},
	}
	tor := &state.TorrentStatus{
		Hash: "abc123",
		FileStats: []*state.TorrentFileStat{
			{ID: 1, Path: "video1.mp4"},
			{ID: 2, Path: "video2.mp4"},
		},
	}

	result := searchLastPlayed(viewedSvc, tor)

	if result != 1 {
		t.Errorf("expected 1 (index of FileIndex 2), got %d", result)
	}
}
