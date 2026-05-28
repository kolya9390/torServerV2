package apiservices

import (
	"net/http"
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

func (s *ensureTorrentServiceStub) Status(tor contracts.TorrentHandle) *contracts.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (s *ensureTorrentServiceStub) StatusByHash(hash string) (*contracts.TorrentStatus, bool) {
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
func (s *ensureTorrentServiceStub) Statuses() []*contracts.TorrentStatus {
	return []*contracts.TorrentStatus{}
}
func (s *ensureTorrentServiceStub) ListHashes() []string { return []string{} }
func (s *ensureTorrentServiceStub) Drop(hash string)     {}
func (s *ensureTorrentServiceStub) IsStored(tor contracts.TorrentHandle) bool {
	return tor != nil && tor.State() == contracts.TorrentInDB
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

type ensureTorrentHandleSpy struct {
	state            contracts.TorrentState
	ready            bool
	metadataCalls    int
	readyCalls       int
	ensureTitleCalls int
}

func (h *ensureTorrentHandleSpy) Status() *contracts.TorrentStatus {
	return &contracts.TorrentStatus{Stat: h.state}
}

func (h *ensureTorrentHandleSpy) State() contracts.TorrentState {
	return h.state
}

func (h *ensureTorrentHandleSpy) HashHex() string {
	return "0102030405060708090a0b0c0d0e0f1011121314"
}

func (h *ensureTorrentHandleSpy) Name() string {
	return "spy"
}

func (h *ensureTorrentHandleSpy) FileCount() int {
	return 1
}

func (h *ensureTorrentHandleSpy) Ready() bool {
	h.readyCalls++

	return h.ready
}

func (h *ensureTorrentHandleSpy) EnsureTitleFromInfo() {
	h.ensureTitleCalls++
}

func (h *ensureTorrentHandleSpy) Metadata() contracts.StreamMeta {
	h.metadataCalls++

	return contracts.StreamMeta{Title: "stored"}
}

func (h *ensureTorrentHandleSpy) Stream(_ int, _ *http.Request, _ http.ResponseWriter) error {
	return nil
}

func TestEnsureTorrent_ActiveTorrentUsesFastPath(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{1, 2, 3, 4}

	activeTorrent := &ensureTorrentHandleSpy{state: contracts.TorrentWorking, ready: true}
	stub := &ensureTorrentServiceStub{getResult: activeTorrent}

	got, err := svc.EnsureTorrent(stub, wrapTorrentSpec(spec), contracts.StreamMeta{}, true)
	if err != nil {
		t.Fatalf("EnsureTorrent returned error: %v", err)
	}

	if got != activeTorrent {
		t.Fatalf("EnsureTorrent returned %p, want %p", got, activeTorrent)
	}

	if stub.addCalls != 0 || stub.loadCalls != 0 {
		t.Fatalf("fast path must not add/load torrents, add=%d load=%d", stub.addCalls, stub.loadCalls)
	}

	if activeTorrent.readyCalls != 0 {
		t.Fatalf("fast path must not wait on Ready, calls=%d", activeTorrent.readyCalls)
	}

	if activeTorrent.metadataCalls != 0 {
		t.Fatalf("fast path must not read metadata, calls=%d", activeTorrent.metadataCalls)
	}

	if activeTorrent.ensureTitleCalls != 1 {
		t.Fatalf("EnsureTitleFromInfo calls = %d, want 1", activeTorrent.ensureTitleCalls)
	}
}

func TestEnsureTorrent_MissingTorrentRequiresCreatePermission(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{4, 3, 2, 1}
	stub := &ensureTorrentServiceStub{}

	_, err := svc.EnsureTorrent(stub, wrapTorrentSpec(spec), contracts.StreamMeta{}, false)
	if err != contracts.ErrStreamUnauthorized {
		t.Fatalf("EnsureTorrent error = %v, want %v", err, contracts.ErrStreamUnauthorized)
	}

	if stub.addCalls != 0 {
		t.Fatalf("Add calls = %d, want 0", stub.addCalls)
	}
}

func TestEnsureTorrent_FirstRequestAddsAndWaitsForInfo(t *testing.T) {
	svc := streamService{}
	spec := &torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{5, 4, 3, 2}
	addedTorrent := &ensureTorrentHandleSpy{state: contracts.TorrentAdded, ready: true}
	stub := &ensureTorrentServiceStub{addResult: addedTorrent}

	got, err := svc.EnsureTorrent(
		stub,
		wrapTorrentSpec(spec),
		contracts.StreamMeta{Title: "title", Poster: "poster", Category: "movie", Data: "data"},
		true,
	)
	if err != nil {
		t.Fatalf("EnsureTorrent returned error: %v", err)
	}

	if got != addedTorrent {
		t.Fatalf("EnsureTorrent returned %p, want %p", got, addedTorrent)
	}

	if stub.addCalls != 1 {
		t.Fatalf("Add calls = %d, want 1", stub.addCalls)
	}

	if addedTorrent.readyCalls != 1 {
		t.Fatalf("Ready calls = %d, want 1", addedTorrent.readyCalls)
	}

	if addedTorrent.ensureTitleCalls != 1 {
		t.Fatalf("EnsureTitleFromInfo calls = %d, want 1", addedTorrent.ensureTitleCalls)
	}
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
