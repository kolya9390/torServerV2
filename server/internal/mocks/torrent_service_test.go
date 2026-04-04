package mocks

import (
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"server/torr"
	"server/torr/state"
)

func TestMockTorrentService_Get(t *testing.T) {
	mockSvc := new(MockTorrentService)

	expectedTorrent := &torr.Torrent{
		Title: "Test Torrent",
		Stat:  state.TorrentWorking,
	}

	mockSvc.On("Get", "testhash123").Return(expectedTorrent)

	result := mockSvc.Get("testhash123")

	require.NotNil(t, result)
	assert.Equal(t, "Test Torrent", result.Title)
	mockSvc.AssertExpectations(t)
}

func TestMockTorrentService_List(t *testing.T) {
	mockSvc := new(MockTorrentService)

	torrents := []*torr.Torrent{
		{Title: "Torrent 1", Stat: state.TorrentWorking},
		{Title: "Torrent 2", Stat: state.TorrentPreload},
	}

	mockSvc.On("List").Return(torrents)

	result := mockSvc.List()

	assert.Len(t, result, 2)
	mockSvc.AssertExpectations(t)
}

func TestMockTorrentService_Add(t *testing.T) {
	mockSvc := new(MockTorrentService)

	spec := &torrent.TorrentSpec{}
	expected := &torr.Torrent{
		Title: "New Torrent",
		Stat:  state.TorrentAdded,
	}

	mockSvc.On("Add", spec, "Title", "", "", "").Return(expected, nil)

	result, err := mockSvc.Add(spec, "Title", "", "", "")

	require.NoError(t, err)
	assert.Equal(t, "New Torrent", result.Title)
	mockSvc.AssertExpectations(t)
}

func BenchmarkMockTorrentService_Get(b *testing.B) {
	mockSvc := new(MockTorrentService)
	tor := &torr.Torrent{Title: "Test"}
	mockSvc.On("Get", "hash").Return(tor)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockSvc.Get("hash")
	}
}
