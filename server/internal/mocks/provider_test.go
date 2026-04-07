package mocks

import (
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"go.uber.org/mock/gomock"

	"server/settings"
	"server/torr"
	"server/torr/state"
)

func TestMockTorrentService_AddTorrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := NewMockTorrentService(ctrl)
	infoHash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	spec := &torrent.TorrentSpec{
		AddTorrentOpts: torrent.AddTorrentOpts{
			InfoHash: infoHash,
		},
	}

	expectedTorr := &torr.Torrent{
		Title: "Test",
		Stat:  state.TorrentWorking,
	}

	mockSvc.EXPECT().
		AddTorrent(spec, "Test", "", "", "").
		Return(expectedTorr, nil)

	got, err := mockSvc.AddTorrent(spec, "Test", "", "", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if got.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", got.Title)
	}
}

func TestMockTorrentService_GetTorrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := NewMockTorrentService(ctrl)
	hash := "abcdef1234567890abcdef1234567890abcdef12"

	expectedTorr := &torr.Torrent{
		Title: "Test",
		Stat:  state.TorrentWorking,
	}

	mockSvc.EXPECT().
		GetTorrent(hash).
		Return(expectedTorr)

	got := mockSvc.GetTorrent(hash)
	if got == nil {
		t.Fatal("Expected torrent to be found")
	}

	if got.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", got.Title)
	}
}

func TestMockTorrentService_RemoveTorrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := NewMockTorrentService(ctrl)
	hash := "abcdef1234567890abcdef1234567890abcdef12"

	mockSvc.EXPECT().RemoveTorrent(hash)

	mockSvc.RemoveTorrent(hash)
}

func TestMockSettingsProvider_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProvider := NewMockSettingsProvider(ctrl)
	expectedSets := &settings.BTSets{CacheSize: 128 * 1024 * 1024}

	mockProvider.EXPECT().
		Get().
		Return(expectedSets)

	got := mockProvider.Get()
	if got == nil {
		t.Fatal("Expected settings to be found")
	}

	if got.CacheSize != 128*1024*1024 {
		t.Errorf("Expected CacheSize 128MB, got %d", got.CacheSize)
	}
}

func TestMockSettingsProvider_Set(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProvider := NewMockSettingsProvider(ctrl)
	newSets := &settings.BTSets{CacheSize: 256 * 1024 * 1024}

	mockProvider.EXPECT().Set(newSets)

	mockProvider.Set(newSets)
}

func TestMockSettingsProvider_ReadOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockProvider := NewMockSettingsProvider(ctrl)

	mockProvider.EXPECT().
		ReadOnly().
		Return(false)

	if mockProvider.ReadOnly() {
		t.Error("Expected ReadOnly to be false")
	}
}
