package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"
	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	sets "server/settings"
	"server/torr"
	"server/torznab"
)

type contractTorrentService struct{ noopTorrentService }

type contractSettingsService struct {
	noopSettingsService
	current    *sets.BTSets
	defaultSet bool
}

type contractModulesService struct {
	noopModulesService
	stopCalled bool
}

type contractSearchService struct {
	noopSearchService
	enabled bool
}

type contractViewedService struct{ noopViewedService }
type contractSystemService struct{ noopSystemService }
type contractMediaService struct{ noopMediaService }

func (s *contractSettingsService) Current() *sets.BTSets {
	if s.current == nil {
		return &sets.BTSets{}
	}

	return s.current
}

func (s *contractSettingsService) SetDefault() {
	s.defaultSet = true
}

func (s *contractModulesService) StopDLNA() {
	s.stopCalled = true
}

func (s *contractSearchService) EnableTorznabSearch() bool {
	return s.enabled
}

func (s *contractSearchService) TorznabSearch(query string, index int) []*torznab.TorrentDetails {
	return []*torznab.TorrentDetails{}
}

func (s *contractMediaService) ProbePlayURL(hash, fileID string) (*goffprobe.ProbeData, error) {
	return nil, nil
}

func (s *contractTorrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	return nil, nil
}

func withServices(t *testing.T, s *APIServices) {
	t.Helper()

	prev := getServices()

	SetServices(s)
	t.Cleanup(func() {
		SetServices(prev)
	})
}

func TestSettingsDefLegacyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingsSvc := &contractSettingsService{}
	modulesSvc := &contractModulesService{}
	withServices(t, &APIServices{
		Torrents: &contractTorrentService{},
		Settings: settingsSvc,
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{},
		Media:    &contractMediaService{},
		Modules:  modulesSvc,
	})

	r := gin.New()
	r.POST("/settings", settings)

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(`{"action":"def"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !settingsSvc.defaultSet {
		t.Fatal("expected SetDefault to be called")
	}

	if !modulesSvc.stopCalled {
		t.Fatal("expected StopDLNA to be called")
	}
}

func TestTorznabSearchDisabledLegacyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	withServices(t, &APIServices{
		Torrents: &contractTorrentService{},
		Settings: &contractSettingsService{},
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{enabled: false},
		Media:    &contractMediaService{},
		Modules:  &contractModulesService{},
	})

	r := gin.New()
	r.GET("/torznab/search", torznabSearch)

	req := httptest.NewRequest(http.MethodGet, "/torznab/search?query=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var body []string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected []string response, got err: %v body=%s", err, w.Body.String())
	}

	if len(body) != 0 {
		t.Fatalf("expected empty list, got %v", body)
	}
}
