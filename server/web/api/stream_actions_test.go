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

	"server/torr/state"
)

type testTorrentHandle struct {
	status *state.TorrentStatus
	state  state.TorrentStat
	hash   string
	name   string
	files  int
}

func (h testTorrentHandle) Status() *state.TorrentStatus {
	if h.status != nil {
		return h.status
	}

	return &state.TorrentStatus{Hash: h.hash, Stat: h.state}
}

func (h testTorrentHandle) State() state.TorrentStat       { return h.state }
func (h testTorrentHandle) HashHex() string                { return h.hash }
func (h testTorrentHandle) Name() string                   { return h.name }
func (h testTorrentHandle) FileCount() int                 { return h.files }
func (h testTorrentHandle) Ready() bool                    { return true }
func (h testTorrentHandle) EnsureTitleFromInfo()           {}
func (h testTorrentHandle) Metadata() contracts.StreamMeta { return contracts.StreamMeta{} }
func (h testTorrentHandle) Stream(index int, request *http.Request, writer http.ResponseWriter) error {
	return nil
}

type testStreamService struct {
	parseLinkErr      error
	parseLinkCalls    int
	parseMeta         contracts.StreamMeta
	ensureTorrentErr  error
	ensureTorrentTor  contracts.TorrentHandle
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

func (m *testStreamService) EnsureTorrent(torrents contracts.TorrentService, spec contracts.TorrentSpec, meta contracts.StreamMeta, allowCreate bool) (contracts.TorrentHandle, error) {
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
		status: &state.TorrentStatus{
			Hash:  spec.HashHex(),
			Title: title,
			Stat:  state.TorrentAdded,
		},
		hash:  spec.HashHex(),
		state: state.TorrentAdded,
		name:  title,
		files: 1,
	}, m.addErr
}

func (m *testTorrentService) Get(hash string) contracts.TorrentHandle {
	return m.getResult
}

func (m *testTorrentService) Status(tor contracts.TorrentHandle) *state.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (m *testTorrentService) StatusByHash(hash string) (*state.TorrentStatus, bool) {
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

func (m *testTorrentService) Statuses() []*state.TorrentStatus {
	stats := make([]*state.TorrentStatus, 0, len(m.listResult))

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
	return tor != nil && tor.State() == state.TorrentInDB
}

func (m *testTorrentService) DropReadiness(hash string) contracts.DropReadiness {
	return m.readiness
}

func (m *testTorrentService) CacheStateByHash(hash string) (any, bool) {
	return nil, false
}

func (m *testTorrentService) EnqueuePreload(tor contracts.TorrentHandle, index int) bool {
	return false
}

func (m *testTorrentService) EnqueueMetadataFinalize(tor contracts.TorrentHandle, spec *contracts.TorrentSpec, saveToDB bool) bool {
	return false
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
