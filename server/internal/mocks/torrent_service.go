package mocks

import (
	"github.com/anacrolix/torrent"
	"github.com/stretchr/testify/mock"

	sets "server/settings"
	"server/torr"
)

type MockTorrentService struct {
	mock.Mock
}

func (m *MockTorrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	args := m.Called(spec, title, poster, data, category)

	var tor *torr.Torrent

	if args.Get(0) != nil {
		tor = args.Get(0).(*torr.Torrent)
	}

	return tor, args.Error(1)
}

func (m *MockTorrentService) Get(hash string) *torr.Torrent {
	args := m.Called(hash)

	var tor *torr.Torrent

	if args.Get(0) != nil {
		tor = args.Get(0).(*torr.Torrent)
	}

	return tor
}

func (m *MockTorrentService) Set(hash, title, poster, category, data string) *torr.Torrent {
	args := m.Called(hash, title, poster, category, data)

	var tor *torr.Torrent

	if args.Get(0) != nil {
		tor = args.Get(0).(*torr.Torrent)
	}

	return tor
}

func (m *MockTorrentService) SaveToDB(tor *torr.Torrent) {
	m.Called(tor)
}

func (m *MockTorrentService) Remove(hash string) {
	m.Called(hash)
}

func (m *MockTorrentService) List() []*torr.Torrent {
	args := m.Called()

	return args.Get(0).([]*torr.Torrent)
}

func (m *MockTorrentService) Drop(hash string) {
	m.Called(hash)
}

func (m *MockTorrentService) EnqueuePreload(tor *torr.Torrent, index int) bool {
	args := m.Called(tor, index)

	return args.Bool(0)
}

func (m *MockTorrentService) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	args := m.Called(tor, spec, saveToDB)

	return args.Bool(0)
}

func (m *MockTorrentService) LoadFromDB(tor *torr.Torrent) *torr.Torrent {
	args := m.Called(tor)

	var result *torr.Torrent

	if args.Get(0) != nil {
		result = args.Get(0).(*torr.Torrent)
	}

	return result
}

type MockSettingsService struct {
	mock.Mock
}

func (m *MockSettingsService) Current() *sets.BTSets {
	args := m.Called()

	var s *sets.BTSets

	if args.Get(0) != nil {
		s = args.Get(0).(*sets.BTSets)
	}

	return s
}

func (m *MockSettingsService) Set(s *sets.BTSets) {
	m.Called(s)
}

func (m *MockSettingsService) SetDefault() {
	m.Called()
}

func (m *MockSettingsService) ReadOnly() bool {
	args := m.Called()

	return args.Bool(0)
}

func (m *MockSettingsService) GetStoragePreferences() map[string]any {
	args := m.Called()

	return args.Get(0).(map[string]any)
}

func (m *MockSettingsService) SetStoragePreferences(prefs map[string]any) error {
	args := m.Called(prefs)

	return args.Error(0)
}

func (m *MockSettingsService) TMDBConfig() (sets.TMDBConfig, bool) {
	args := m.Called()

	return args.Get(0).(sets.TMDBConfig), args.Bool(1)
}

func (m *MockSettingsService) BuildPlayURL(hash, fileID string) string {
	args := m.Called(hash, fileID)

	return args.String(0)
}

func (m *MockSettingsService) EnableDLNA() bool {
	args := m.Called()

	return args.Bool(0)
}

func (m *MockSettingsService) EnableDebug() bool {
	args := m.Called()

	return args.Bool(0)
}

type Settings = sets.BTSets
