package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"

	"server/torr"
)

type testStreamService struct {
	parseLinkErr      error
	ensureTorrentErr  error
	ensureTorrentTor  *torr.Torrent
	parseFileIndexVal int
	parseFileIndexErr error
	normalizeResult   string
}

func (m *testStreamService) ParseLink(link, title, poster, category string) (*torrent.TorrentSpec, StreamMeta, error) {
	if m.parseLinkErr != nil {
		return nil, StreamMeta{}, m.parseLinkErr
	}

	spec := torrent.TorrentSpec{}
	spec.InfoHash = torrent.InfoHash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

	return &spec, StreamMeta{}, nil
}

func (m *testStreamService) EnsureTorrent(torrents TorrentService, spec *torrent.TorrentSpec, meta StreamMeta, allowCreate bool) (*torr.Torrent, error) {
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
	getResult      *torr.Torrent
	listResult     []*torr.Torrent
	addErr         error
	saveToDBCalled bool
}

func (m *testTorrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	return nil, m.addErr
}

func (m *testTorrentService) Get(hash string) *torr.Torrent {
	return m.getResult
}

func (m *testTorrentService) Set(hash, title, poster, category, data string) *torr.Torrent {
	return nil
}

func (m *testTorrentService) SaveToDB(tor *torr.Torrent) {
	m.saveToDBCalled = true
}

func (m *testTorrentService) Remove(hash string) {}

func (m *testTorrentService) List() []*torr.Torrent {
	return m.listResult
}

func (m *testTorrentService) Drop(hash string) {}

func (m *testTorrentService) EnqueuePreload(tor *torr.Torrent, index int) bool {
	return false
}

func (m *testTorrentService) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	return false
}

func (m *testTorrentService) LoadFromDB(tor *torr.Torrent) *torr.Torrent {
	return nil
}

func TestStreamPlayValidationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
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

	svc := &APIServices{
		Torrents: torrentsSvc,
		Streams:  streamSvc,
	}
	SetServices(svc)

	defer SetServices(nil)

	r := gin.New()
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

	svc := &APIServices{
		Streams: &testStreamService{
			parseLinkErr: ErrStreamLinkEmpty,
		},
	}
	SetServices(svc)

	defer SetServices(nil)

	r := gin.New()
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
		{"unauthorized", ErrStreamUnauthorized, http.StatusUnauthorized},
		{"timeout", ErrStreamConnectionTimeout, http.StatusInternalServerError},
		{"other error", ErrStreamInvalidLink, http.StatusInternalServerError},
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

	svc := &APIServices{
		Streams: &testStreamService{
			parseLinkErr: ErrStreamLinkEmpty,
		},
	}
	SetServices(svc)

	defer SetServices(nil)

	r := gin.New()
	r.GET("/streams/play", streamPlay)

	req := httptest.NewRequest(http.MethodGet, "/streams/play?link=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
