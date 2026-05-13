package apiservices

import (
	"testing"

	"github.com/anacrolix/torrent"

	"server/settings"
	"server/torr"
	"server/torr/state"
	"server/web/api"
)

type ensureTorrentServiceStub struct {
	getResult  *torr.Torrent
	loadResult *torr.Torrent
	addResult  *torr.Torrent
	addErr     error

	loadCalls int
	addCalls  int
}

func (s *ensureTorrentServiceStub) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	s.addCalls++

	return s.addResult, s.addErr
}

func (s *ensureTorrentServiceStub) Get(hash string) *torr.Torrent {
	return s.getResult
}

func (s *ensureTorrentServiceStub) Set(hash, title, poster, category, data string) *torr.Torrent {
	return nil
}

func (s *ensureTorrentServiceStub) SaveToDB(tor *torr.Torrent) {}
func (s *ensureTorrentServiceStub) Remove(hash string)         {}
func (s *ensureTorrentServiceStub) List() []*torr.Torrent      { return nil }
func (s *ensureTorrentServiceStub) Drop(hash string)           {}
func (s *ensureTorrentServiceStub) EnqueuePreload(tor *torr.Torrent, index int) bool {
	return false
}

func (s *ensureTorrentServiceStub) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	return false
}

func (s *ensureTorrentServiceStub) LoadFromDB(tor *torr.Torrent) *torr.Torrent {
	s.loadCalls++

	return s.loadResult
}

func TestEnsureTorrent_LoadsDBTorrentBeforePlayback(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{1, 2, 3}

	dbTorrent := &torr.Torrent{Stat: state.TorrentInDB, Title: "stored"}
	loadedTorrent := &torr.Torrent{Stat: state.TorrentWorking, Title: "stored"}
	stub := &ensureTorrentServiceStub{
		getResult:  dbTorrent,
		loadResult: loadedTorrent,
	}

	got, err := svc.EnsureTorrent(stub, spec, api.StreamMeta{}, true)
	if err != nil {
		t.Fatalf("EnsureTorrent returned error: %v", err)
	}

	if got != loadedTorrent {
		t.Fatalf("EnsureTorrent returned %p, want %p", got, loadedTorrent)
	}

	if stub.loadCalls != 1 {
		t.Fatalf("LoadFromDB calls = %d, want 1", stub.loadCalls)
	}

	if stub.addCalls != 0 {
		t.Fatalf("Add calls = %d, want 0", stub.addCalls)
	}
}

func TestEnsureTorrent_DBTorrentRequiresActivationPermission(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{9, 9, 9}

	stub := &ensureTorrentServiceStub{
		getResult: &torr.Torrent{Stat: state.TorrentInDB},
	}

	_, err := svc.EnsureTorrent(stub, spec, api.StreamMeta{}, false)
	if err != api.ErrStreamUnauthorized {
		t.Fatalf("EnsureTorrent error = %v, want %v", err, api.ErrStreamUnauthorized)
	}

	if stub.loadCalls != 0 {
		t.Fatalf("LoadFromDB calls = %d, want 0", stub.loadCalls)
	}

	if stub.addCalls != 0 {
		t.Fatalf("Add calls = %d, want 0", stub.addCalls)
	}
}

func TestResolveDefaultDeps_UsesNoopSettingsProvider(t *testing.T) {
	resolved := resolveDefaultDeps(DefaultDeps{})
	if resolved.SettingsProvider == nil {
		t.Fatal("SettingsProvider is nil")
	}

	if resolved.SettingsProvider == settings.DefaultSettingsProvider {
		t.Fatal("SettingsProvider unexpectedly bound to DefaultSettingsProvider")
	}

	if got := resolved.SettingsProvider.Get(); got == nil {
		t.Fatal("SettingsProvider.Get() returned nil")
	}
}
