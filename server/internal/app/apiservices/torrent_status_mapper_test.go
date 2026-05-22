package apiservices

import (
	"reflect"
	"testing"

	"server/internal/app/contracts"
	"server/torr/state"
)

func TestMapTorrentStatusUsesApplicationDTO(t *testing.T) {
	src := &state.TorrentStatus{
		Title:               "Movie",
		Category:            "Films",
		Poster:              "poster.jpg",
		Data:                "metadata",
		Timestamp:           42,
		Name:                "movie.mkv",
		Hash:                "abc",
		TorrsHash:           "torrs",
		Stat:                state.TorrentWorking,
		StatString:          "Torrent working",
		LoadedSize:          10,
		TorrentSize:         20,
		PreloadedBytes:      3,
		PreloadSize:         4,
		DownloadSpeed:       5.5,
		UploadSpeed:         1.5,
		TotalPeers:          7,
		PendingPeers:        2,
		ActivePeers:         3,
		ConnectedSeeders:    4,
		HalfOpenPeers:       5,
		BytesWritten:        6,
		BytesWrittenData:    7,
		BytesRead:           8,
		BytesReadData:       9,
		BytesReadUsefulData: 10,
		ChunksWritten:       11,
		ChunksRead:          12,
		ChunksReadUseful:    13,
		ChunksReadWasted:    14,
		PiecesDirtiedGood:   15,
		PiecesDirtiedBad:    16,
		DurationSeconds:     17.5,
		BitRate:             "1 Mbps",
		FileStats: []*state.TorrentFileStat{
			nil,
			{ID: 1, Path: "movie.mkv", Length: 1024},
		},
	}

	got := mapTorrentStatus(src)
	want := &contracts.TorrentStatus{
		Title:               "Movie",
		Category:            "Films",
		Poster:              "poster.jpg",
		Data:                "metadata",
		Timestamp:           42,
		Name:                "movie.mkv",
		Hash:                "abc",
		TorrsHash:           "torrs",
		Stat:                contracts.TorrentWorking,
		StatString:          "Torrent working",
		LoadedSize:          10,
		TorrentSize:         20,
		PreloadedBytes:      3,
		PreloadSize:         4,
		DownloadSpeed:       5.5,
		UploadSpeed:         1.5,
		TotalPeers:          7,
		PendingPeers:        2,
		ActivePeers:         3,
		ConnectedSeeders:    4,
		HalfOpenPeers:       5,
		BytesWritten:        6,
		BytesWrittenData:    7,
		BytesRead:           8,
		BytesReadData:       9,
		BytesReadUsefulData: 10,
		ChunksWritten:       11,
		ChunksRead:          12,
		ChunksReadUseful:    13,
		ChunksReadWasted:    14,
		PiecesDirtiedGood:   15,
		PiecesDirtiedBad:    16,
		DurationSeconds:     17.5,
		BitRate:             "1 Mbps",
		FileStats: []*contracts.TorrentFile{
			{ID: 1, Path: "movie.mkv", Length: 1024},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected torrent status:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMapTorrentStatusReturnsNilForNilInput(t *testing.T) {
	if got := mapTorrentStatus(nil); got != nil {
		t.Fatalf("expected nil torrent status, got %#v", got)
	}

	if got := mapTorrentFiles(nil); got != nil {
		t.Fatalf("expected nil torrent files, got %#v", got)
	}
}
