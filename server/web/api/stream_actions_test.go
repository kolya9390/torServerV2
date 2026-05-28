package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"server/internal/app/contracts"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type testTorrentHandle struct {
	status *contracts.TorrentStatus
	state  contracts.TorrentState
	hash   string
	name   string
	files  int
}

func (h testTorrentHandle) Status() *contracts.TorrentStatus {
	if h.status != nil {
		return h.status
	}

	return &contracts.TorrentStatus{Hash: h.hash, Stat: h.state}
}

func (h testTorrentHandle) State() contracts.TorrentState  { return h.state }
func (h testTorrentHandle) HashHex() string                { return h.hash }
func (h testTorrentHandle) Name() string                   { return h.name }
func (h testTorrentHandle) FileCount() int                 { return h.files }
func (h testTorrentHandle) Ready() bool                    { return true }
func (h testTorrentHandle) EnsureTitleFromInfo()           {}
func (h testTorrentHandle) Metadata() contracts.StreamMeta { return contracts.StreamMeta{} }
func (h testTorrentHandle) Stream(index int, request *http.Request, writer http.ResponseWriter) error {
	return nil
}

type recordingTorrentHandle struct {
	status      *contracts.TorrentStatus
	state       contracts.TorrentState
	hash        string
	name        string
	files       int
	streamCalls int
	streamIndex int
	streamRange string
}

func (h *recordingTorrentHandle) Status() *contracts.TorrentStatus {
	if h.status != nil {
		return h.status
	}

	return &contracts.TorrentStatus{Hash: h.hash, Stat: h.state}
}

func (h *recordingTorrentHandle) State() contracts.TorrentState { return h.state }
func (h *recordingTorrentHandle) HashHex() string               { return h.hash }
func (h *recordingTorrentHandle) Name() string                  { return h.name }
func (h *recordingTorrentHandle) FileCount() int                { return h.files }
func (h *recordingTorrentHandle) Ready() bool                   { return true }
func (h *recordingTorrentHandle) EnsureTitleFromInfo()          {}
func (h *recordingTorrentHandle) Metadata() contracts.StreamMeta {
	return contracts.StreamMeta{}
}

func (h *recordingTorrentHandle) Stream(index int, request *http.Request, writer http.ResponseWriter) error {
	h.streamCalls++
	h.streamIndex = index
	h.streamRange = request.Header.Get("Range")

	return nil
}

type testStreamService struct {
	parseLinkErr      error
	parseLinkCalls    int
	parseMeta         contracts.StreamMeta
	ensureTorrentErr  error
	ensureTorrentTor  contracts.TorrentHandle
	ensureAllowCreate bool
	parseFileIndexVal int
	parseFileIndexErr error
	normalizeResult   string
}

func (m *testStreamService) ParseLink(link, title, poster, category string) (contracts.TorrentSpec, contracts.StreamMeta, error) {
	m.parseLinkCalls++

	if m.parseLinkErr != nil {
		return contracts.TorrentSpec{}, contracts.StreamMeta{}, m.parseLinkErr
	}

	return contracts.NewTorrentSpec("0102030405060708090a0b0c0d0e0f1011121314", nil), m.parseMeta, nil
}

func (m *testStreamService) ParseTorrentFile(reader io.Reader) (contracts.TorrentSpec, error) {
	return contracts.NewTorrentSpec("0102030405060708090a0b0c0d0e0f1011121314", nil), nil
}

func (m *testStreamService) EnsureTorrent(torrents contracts.TorrentStreamService, spec contracts.TorrentSpec, meta contracts.StreamMeta, allowCreate bool) (contracts.TorrentHandle, error) {
	m.ensureAllowCreate = allowCreate

	if m.ensureTorrentErr != nil {
		return nil, m.ensureTorrentErr
	}

	return m.ensureTorrentTor, nil
}

func (m *testStreamService) ParseFileIndex(index string, fileCount int) (int, error) {
	if m.parseFileIndexErr != nil {
		return 0, m.parseFileIndexErr
	}

	return m.parseFileIndexVal, nil
}

func (m *testStreamService) NormalizePlaylistName(rawName, fallback string) string {
	if m.normalizeResult != "" {
		return m.normalizeResult
	}

	return fallback + ".m3u"
}

type testTorrentService struct {
	getResult      contracts.TorrentHandle
	listResult     []contracts.TorrentHandle
	addErr         error
	addResult      contracts.TorrentHandle
	addCalls       int
	addTitle       string
	addPoster      string
	addData        string
	addCategory    string
	saveToDBCalled bool
	dropCalled     bool
	readiness      contracts.DropReadiness
	preloadResult  bool
	preloadCalled  bool
	preloadIndex   int

	finalizeCalled bool
	finalizeTor    contracts.TorrentHandle
	finalizeSpec   *contracts.TorrentSpec
	finalizeSave   bool
}

func (m *testTorrentService) Add(spec contracts.TorrentSpec, title, poster, data, category string) (contracts.TorrentHandle, error) {
	m.addCalls++
	m.addTitle = title
	m.addPoster = poster
	m.addData = data
	m.addCategory = category

	if m.addResult != nil {
		return m.addResult, m.addErr
	}

	return testTorrentHandle{
		status: &contracts.TorrentStatus{
			Hash:  spec.HashHex(),
			Title: title,
			Stat:  contracts.TorrentAdded,
		},
		hash:  spec.HashHex(),
		state: contracts.TorrentAdded,
		name:  title,
		files: 1,
	}, m.addErr
}

func (m *testTorrentService) Get(hash string) contracts.TorrentHandle {
	return m.getResult
}

func (m *testTorrentService) Status(tor contracts.TorrentHandle) *contracts.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (m *testTorrentService) StatusByHash(hash string) (*contracts.TorrentStatus, bool) {
	if m.getResult == nil {
		return nil, false
	}

	return m.getResult.Status(), true
}

func (m *testTorrentService) Set(hash, title, poster, category, data string) contracts.TorrentHandle {
	return nil
}

func (m *testTorrentService) SaveToDB(tor contracts.TorrentHandle) {
	m.saveToDBCalled = true
}

func (m *testTorrentService) Remove(hash string) {}

func (m *testTorrentService) List() []contracts.TorrentHandle {
	return m.listResult
}

func (m *testTorrentService) Statuses() []*contracts.TorrentStatus {
	stats := make([]*contracts.TorrentStatus, 0, len(m.listResult))

	for _, tor := range m.listResult {
		if tor != nil {
			stats = append(stats, tor.Status())
		}
	}

	return stats
}

func (m *testTorrentService) ListHashes() []string {
	return []string{}
}

func (m *testTorrentService) Drop(hash string) {
	m.dropCalled = true
}

func (m *testTorrentService) IsStored(tor contracts.TorrentHandle) bool {
	return tor != nil && tor.State() == contracts.TorrentInDB
}

func (m *testTorrentService) DropReadiness(hash string) contracts.DropReadiness {
	return m.readiness
}

func (m *testTorrentService) CacheStateByHash(hash string) (any, bool) {
	return nil, false
}

func (m *testTorrentService) EnqueuePreload(tor contracts.TorrentHandle, index int) bool {
	m.preloadCalled = true
	m.preloadIndex = index

	return m.preloadResult
}

func (m *testTorrentService) EnqueueMetadataFinalize(tor contracts.TorrentHandle, spec *contracts.TorrentSpec, saveToDB bool) bool {
	m.finalizeCalled = true
	m.finalizeTor = tor
	m.finalizeSpec = spec
	m.finalizeSave = saveToDB

	return true
}

func (m *testTorrentService) LoadFromDB(tor contracts.TorrentHandle) contracts.TorrentHandle {
	return nil
}

func TestStreamPlayValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"link"`) {
		t.Fatalf("expected link validation error, got %s", w.Body.String())
	}
}

func TestStreamStatValidationLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
	r.GET("/streams/stat", streamStat)

	req := httptest.NewRequest(http.MethodGet, "/streams/stat?link=not-a-link", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"link"`) {
		t.Fatalf("expected link validation error, got %s", w.Body.String())
	}
}

func TestStreamStatNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &testTorrentService{getResult: nil}
	streamSvc := &testStreamService{}

	svc := &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, svc)))
	r.GET("/streams/stat", streamStat)

	req := httptest.NewRequest(http.MethodGet, "/streams/stat?link=magnet:?xt=urn:btih:abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestStreamServiceParseLinkError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &contracts.APIServices{
		Streams: &testStreamService{
			parseLinkErr: contracts.ErrStreamLinkEmpty,
		},
	}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, svc)))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestStreamPlayFirstRequestStreamsTorrent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := &recordingTorrentHandle{
		hash:  "0102030405060708090a0b0c0d0e0f1011121314",
		name:  "Demo",
		files: 2,
	}
	streamSvc := &testStreamService{ensureTorrentTor: tor, parseFileIndexVal: 1}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: &testTorrentService{getResult: tor},
		Streams:  streamSvc,
	})))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=magnet:?xt=urn:btih:abc123&index=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if tor.streamCalls != 1 || tor.streamIndex != 1 {
		t.Fatalf("expected one stream call for index 1, calls=%d index=%d", tor.streamCalls, tor.streamIndex)
	}

	if !streamSvc.ensureAllowCreate {
		t.Fatal("expected authenticated first play request to allow torrent creation")
	}
}

func TestStreamPlayRepeatedRangeRequestStreamsWithoutPreload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := &recordingTorrentHandle{
		hash:  "0102030405060708090a0b0c0d0e0f1011121314",
		name:  "Demo",
		files: 2,
	}
	torrentsSvc := &testTorrentService{getResult: tor}
	streamSvc := &testStreamService{ensureTorrentTor: tor, parseFileIndexVal: 1}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	})))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=magnet:?xt=urn:btih:abc123&index=1", nil)
	w := httptest.NewRecorder()

	req.Header.Set("Range", "bytes=1048576-2097151")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if tor.streamCalls != 1 || tor.streamRange != "bytes=1048576-2097151" {
		t.Fatalf("expected range stream call, calls=%d range=%q", tor.streamCalls, tor.streamRange)
	}

	if torrentsSvc.preloadCalled {
		t.Fatal("repeated range request without preload flag must not enqueue preload")
	}
}

func TestStreamPlayInvalidIndexDoesNotStream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := &recordingTorrentHandle{
		hash:  "0102030405060708090a0b0c0d0e0f1011121314",
		name:  "Demo",
		files: 2,
	}
	streamSvc := &testStreamService{
		ensureTorrentTor:  tor,
		parseFileIndexErr: contracts.ErrStreamFileIndexInvalid,
	}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: &testTorrentService{getResult: tor},
		Streams:  streamSvc,
	})))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=magnet:?xt=urn:btih:abc123&index=bad", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	if tor.streamCalls != 0 {
		t.Fatalf("invalid index must not stream, calls=%d", tor.streamCalls)
	}
}

func TestIsNotAuthRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		authReq    bool
		authUser   string
		expectAuth bool
	}{
		{"no auth required", false, "", false},
		{"auth required but no user", true, "", true},
		{"auth required with user", true, "admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("auth_required", tt.authReq)

			if tt.authUser != "" {
				c.Set(gin.AuthUserKey, tt.authUser)
			}

			result := isNotAuthRequest(c)
			if result != tt.expectAuth {
				t.Errorf("isNotAuthRequest() = %v, want %v", result, tt.expectAuth)
			}
		})
	}
}

func TestMapStreamEnsureError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"unauthorized", contracts.ErrStreamUnauthorized, http.StatusUnauthorized},
		{"timeout", contracts.ErrStreamConnectionTimeout, http.StatusInternalServerError},
		{"other error", contracts.ErrStreamInvalidLink, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := mapStreamEnsureError(tt.err)
			if status != tt.wantStatus {
				t.Errorf("mapStreamEnsureError() status = %d, want %d", status, tt.wantStatus)
			}
		})
	}
}

func TestStreamServiceParseLink(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &contracts.APIServices{
		Streams: &testStreamService{
			parseLinkErr: contracts.ErrStreamLinkEmpty,
		},
	}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, svc)))
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLegacyStreamStatSaveCombinationKeepsCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := testTorrentHandle{
		status: &contracts.TorrentStatus{Hash: "0102030405060708090a0b0c0d0e0f1011121314", Title: "Demo"},
		hash:   "0102030405060708090a0b0c0d0e0f1011121314",
		name:   "Demo",
		files:  1,
	}
	torrentsSvc := &testTorrentService{getResult: tor}
	streamSvc := &testStreamService{ensureTorrentTor: tor}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	})))
	r.GET("/stream", stream)

	req := httptest.NewRequest(http.MethodGet, "/stream?link=magnet:?xt=urn:btih:abc123&stat&save", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if !torrentsSvc.saveToDBCalled {
		t.Fatal("expected legacy stat+save combination to save torrent before returning status")
	}

	if !strings.Contains(w.Body.String(), `"hash":"0102030405060708090a0b0c0d0e0f1011121314"`) {
		t.Fatalf("expected torrent status response, got %s", w.Body.String())
	}
}

func TestLegacyStreamUnauthenticatedM3UUsesReadOnlyActivation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := testTorrentHandle{
		status: &contracts.TorrentStatus{
			Hash: "0102030405060708090a0b0c0d0e0f1011121314",
			FileStats: []*contracts.TorrentFile{
				{ID: 1, Path: "demo.mp4"},
			},
		},
		hash:  "0102030405060708090a0b0c0d0e0f1011121314",
		name:  "Demo",
		files: 1,
	}
	streamSvc := &testStreamService{ensureTorrentTor: tor}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth_required", true)
	})
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: &testTorrentService{getResult: tor},
		Streams:  streamSvc,
		Playback: playbackStub{m3uResult: "#EXTM3U\n#EXTINF:0,Demo\nhttp://example.test/stream/demo.mp4\n"},
	})))
	r.GET("/stream", stream)

	req := httptest.NewRequest(http.MethodGet, "/stream?link=magnet:?xt=urn:btih:abc123&m3u", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if streamSvc.ensureAllowCreate {
		t.Fatal("expected unauthenticated legacy m3u to avoid creating torrents")
	}

	if !strings.Contains(w.Body.String(), "#EXTM3U") {
		t.Fatalf("expected m3u body, got %s", w.Body.String())
	}
}

func TestLegacyStreamPreloadOnlyReturnsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := testTorrentHandle{
		status: &contracts.TorrentStatus{Hash: "0102030405060708090a0b0c0d0e0f1011121314", Title: "Demo"},
		hash:   "0102030405060708090a0b0c0d0e0f1011121314",
		name:   "Demo",
		files:  2,
	}
	torrentsSvc := &testTorrentService{getResult: tor, preloadResult: true}
	streamSvc := &testStreamService{ensureTorrentTor: tor, parseFileIndexVal: 1}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	})))
	r.GET("/stream", stream)

	req := httptest.NewRequest(http.MethodGet, "/stream?link=magnet:?xt=urn:btih:abc123&index=1&preload", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
	}

	if !torrentsSvc.preloadCalled || torrentsSvc.preloadIndex != 1 {
		t.Fatalf("expected preload queue with index 1, called=%v index=%d", torrentsSvc.preloadCalled, torrentsSvc.preloadIndex)
	}

	if !strings.Contains(w.Body.String(), `"status":"preload accepted"`) {
		t.Fatalf("expected accepted response, got %s", w.Body.String())
	}
}

func TestLegacyStreamPreloadQueueFullReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tor := testTorrentHandle{
		status: &contracts.TorrentStatus{Hash: "0102030405060708090a0b0c0d0e0f1011121314", Title: "Demo"},
		hash:   "0102030405060708090a0b0c0d0e0f1011121314",
		name:   "Demo",
		files:  1,
	}
	torrentsSvc := &testTorrentService{getResult: tor, preloadResult: false}
	streamSvc := &testStreamService{ensureTorrentTor: tor}

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	})))
	r.GET("/stream", stream)

	req := httptest.NewRequest(http.MethodGet, "/stream?link=magnet:?xt=urn:btih:abc123&preload", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}

	if !torrentsSvc.preloadCalled {
		t.Fatal("expected preload queue attempt")
	}
}

func TestDropTorrentRejectsActiveStreamingState(t *testing.T) {
	tests := []struct {
		name      string
		readiness contracts.DropReadiness
	}{
		{
			name: "active readers",
			readiness: contracts.DropReadiness{
				ActiveReaders:       1,
				RecentStreamElapsed: 10 * time.Second,
			},
		},
		{
			name: "active stream sessions",
			readiness: contracts.DropReadiness{
				ActiveStreams:       1,
				RecentStreamElapsed: 10 * time.Second,
			},
		},
		{
			name: "recent stream activity",
			readiness: contracts.DropReadiness{
				RecentStreamElapsed: time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			torrentsSvc := &testTorrentService{readiness: tt.readiness}
			r := gin.New()
			r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{Torrents: torrentsSvc})))
			r.POST("/torrents", torrents)

			req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{"action":"drop","hash":"abc"}`))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
			}

			if torrentsSvc.dropCalled {
				t.Fatal("drop must not be called while streaming is active or reconnecting")
			}
		})
	}
}

func TestDropTorrentAllowsIdleTorrent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &testTorrentService{
		readiness: contracts.DropReadiness{RecentStreamElapsed: 10 * time.Second},
	}
	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{Torrents: torrentsSvc})))
	r.POST("/torrents", torrents)

	req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{"action":"drop","hash":"abc"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if !torrentsSvc.dropCalled {
		t.Fatal("expected idle torrent to be dropped")
	}
}
