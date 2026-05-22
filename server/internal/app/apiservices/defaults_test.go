package apiservices

import (
	"strings"
	"testing"

	"github.com/anacrolix/torrent"

	"server/internal/app/contracts"
	"server/torr"
	"server/torr/state"
)

type ensureTorrentServiceStub struct {
	getResult  contracts.TorrentHandle
	loadResult contracts.TorrentHandle
	addResult  contracts.TorrentHandle
	addErr     error

	loadCalls int
	addCalls  int
}

func (s *ensureTorrentServiceStub) Add(spec contracts.TorrentSpec, title, poster, data, category string) (contracts.TorrentHandle, error) {
	s.addCalls++

	return s.addResult, s.addErr
}

func (s *ensureTorrentServiceStub) Get(hash string) contracts.TorrentHandle {
	return s.getResult
}

func (s *ensureTorrentServiceStub) Status(tor contracts.TorrentHandle) *state.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (s *ensureTorrentServiceStub) StatusByHash(hash string) (*state.TorrentStatus, bool) {
	if s.getResult == nil {
		return nil, false
	}

	return s.getResult.Status(), true
}

func (s *ensureTorrentServiceStub) Set(hash, title, poster, category, data string) contracts.TorrentHandle {
	return nil
}

func (s *ensureTorrentServiceStub) SaveToDB(tor contracts.TorrentHandle) {}
func (s *ensureTorrentServiceStub) Remove(hash string)                   {}
func (s *ensureTorrentServiceStub) List() []contracts.TorrentHandle      { return nil }
func (s *ensureTorrentServiceStub) Statuses() []*state.TorrentStatus {
	return []*state.TorrentStatus{}
}
func (s *ensureTorrentServiceStub) ListHashes() []string { return []string{} }
func (s *ensureTorrentServiceStub) Drop(hash string)     {}
func (s *ensureTorrentServiceStub) IsStored(tor contracts.TorrentHandle) bool {
	return tor != nil && tor.State() == state.TorrentInDB
}
func (s *ensureTorrentServiceStub) DropReadiness(hash string) contracts.DropReadiness {
	return contracts.DropReadiness{}
}
func (s *ensureTorrentServiceStub) CacheStateByHash(hash string) (any, bool) {
	return nil, false
}
func (s *ensureTorrentServiceStub) EnqueuePreload(tor contracts.TorrentHandle, index int) bool {
	return false
}

func (s *ensureTorrentServiceStub) EnqueueMetadataFinalize(tor contracts.TorrentHandle, spec *contracts.TorrentSpec, saveToDB bool) bool {
	return false
}

func (s *ensureTorrentServiceStub) LoadFromDB(tor contracts.TorrentHandle) contracts.TorrentHandle {
	s.loadCalls++

	return s.loadResult
}

func TestEnsureTorrent_LoadsDBTorrentBeforePlayback(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{1, 2, 3}

	dbTorrent := wrapTorrent(&torr.Torrent{Stat: state.TorrentInDB, Title: "stored"})
	loadedTorrent := wrapTorrent(&torr.Torrent{Stat: state.TorrentWorking, Title: "stored"})
	stub := &ensureTorrentServiceStub{
		getResult:  dbTorrent,
		loadResult: loadedTorrent,
	}

	got, err := svc.EnsureTorrent(stub, wrapTorrentSpec(spec), contracts.StreamMeta{}, true)
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
		getResult: wrapTorrent(&torr.Torrent{Stat: state.TorrentInDB}),
	}

	_, err := svc.EnsureTorrent(stub, wrapTorrentSpec(spec), contracts.StreamMeta{}, false)
	if err != contracts.ErrStreamUnauthorized {
		t.Fatalf("EnsureTorrent error = %v, want %v", err, contracts.ErrStreamUnauthorized)
	}

	if stub.loadCalls != 0 {
		t.Fatalf("LoadFromDB calls = %d, want 0", stub.loadCalls)
	}

	if stub.addCalls != 0 {
		t.Fatalf("Add calls = %d, want 0", stub.addCalls)
	}
}

func TestNewDefaultWithDepsReturnsErrorOnMissingRequiredDeps(t *testing.T) {
	services, err := NewDefaultWithDeps(DefaultDeps{})
	if err == nil {
		t.Fatal("expected NewDefaultWithDeps to return missing dependencies error")
	}

	if services != nil {
		t.Fatalf("services = %v, want nil", services)
	}

	message := err.Error()
	if !strings.Contains(message, "TorrentBackend") || !strings.Contains(message, "SettingsProvider") {
		t.Fatalf("expected missing dependencies in error, got %q", message)
	}
}

func TestNewDefaultForTestsUsesExplicitNoopFallbacks(t *testing.T) {
	services, err := NewDefaultForTests(DefaultDeps{})
	if err != nil {
		t.Fatalf("NewDefaultForTests returned error: %v", err)
	}

	if services == nil {
		t.Fatal("services is nil")
	}

	if services.Settings == nil {
		t.Fatal("Settings service is nil")
	}

	if got := services.Settings.Current(); got == nil {
		t.Fatal("Settings.Current() returned nil")
	}
}
