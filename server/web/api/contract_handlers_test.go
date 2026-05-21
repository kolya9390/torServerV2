package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"server/internal/app/contracts"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	sets "server/settings"
	"server/torznab"
	wauth "server/web/auth"
)

type contractTorrentService struct{ mockTorrentService }

type contractSettingsService struct {
	mockSettingsService
	current    *sets.BTSets
	defaultSet bool
	enableDLNA bool
}

type contractModulesService struct {
	mockModulesService
	stopCalled    bool
	restartCalled bool
}

type contractSearchService struct {
	mockSearchService
	enabled bool
}

type contractViewedService struct{ mockViewedService }
type contractSystemService struct{ mockSystemService }
type contractMediaService struct{ mockMediaService } //nolint:unused // embedded to document interface contract

func (s *contractSettingsService) Current() *sets.BTSets {
	if s.current == nil {
		return &sets.BTSets{}
	}

	return s.current
}

func (s *contractSettingsService) SetDefault() {
	s.defaultSet = true
}

func (s *contractSettingsService) EnableDLNA() bool {
	return s.enableDLNA
}

func (s *contractModulesService) StopDLNA() {
	s.stopCalled = true
}

func (s *contractModulesService) RestartDLNA(enable bool) error {
	s.restartCalled = true

	return nil
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

func withServices(t *testing.T, r *gin.Engine, s *contracts.APIServices) {
	t.Helper()

	r.Use(servicesMiddleware(newAPIServicesFixture(t, s)))
}

func TestSettingsDefLegacyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingsSvc := &contractSettingsService{}
	modulesSvc := &contractModulesService{}
	r := gin.New()
	withServices(t, r, &contracts.APIServices{
		Torrents: &contractTorrentService{},
		Settings: settingsSvc,
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{},
		Media:    &contractMediaService{},
		Modules:  modulesSvc,
	})

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

func TestSetupRouteWithServicesUsesScopedServices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	settingsSvc := &contractSettingsService{}
	modulesSvc := &contractModulesService{}

	r := gin.New()
	r.Use(wauth.RuntimeMiddleware(wauth.NewRuntimeFromStore(nil, nil, false)))
	services := newAPIServicesFixture(t, nil)
	services.Torrents = &contractTorrentService{}
	services.Settings = settingsSvc
	services.Viewed = &contractViewedService{}
	services.System = &contractSystemService{}
	services.Search = &contractSearchService{}
	services.Media = &contractMediaService{}
	services.Modules = modulesSvc
	SetupRouteWithServices(r, func() sets.RuntimeState { return sets.RuntimeState{} }, services)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(`{"action":"def"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if !settingsSvc.defaultSet {
		t.Fatal("expected scoped SettingsService.SetDefault to be called")
	}

	if !modulesSvc.stopCalled {
		t.Fatal("expected scoped ModulesService.StopDLNA to be called")
	}
}

func TestSetupRouteWithServicesRequiresCompleteDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected SetupRouteWithServices to panic on incomplete services")
		}

		if !strings.Contains(recovered.(string), "missing") {
			t.Fatalf("expected missing dependency panic, got %v", recovered)
		}
	}()

	SetupRouteWithServices(gin.New(), func() sets.RuntimeState { return sets.RuntimeState{} }, &contracts.APIServices{})
}

func TestServicesMiddlewareRequiresCompleteServices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected servicesMiddleware to panic on incomplete services")
		}

		if !strings.Contains(recovered.(string), "missing") {
			t.Fatalf("expected missing dependency panic, got %v", recovered)
		}
	}()

	_ = servicesMiddleware(&contracts.APIServices{})
}

func TestTorznabSearchDisabledLegacyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	withServices(t, r, &contracts.APIServices{
		Torrents: &contractTorrentService{},
		Settings: &contractSettingsService{},
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{enabled: false},
		Media:    &contractMediaService{},
		Modules:  &contractModulesService{},
	})

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

func TestTorrentsAddDelegatesLinkParsingToStreamService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &testTorrentService{}
	streamSvc := &testStreamService{
		parseMeta: contracts.StreamMeta{
			Title:    "parsed-title",
			Poster:   "parsed-poster",
			Category: "parsed-category",
		},
	}
	settingsSvc := &contractSettingsService{enableDLNA: true}
	modulesSvc := &contractModulesService{}

	r := gin.New()
	withServices(t, r, &contracts.APIServices{
		Torrents: torrentsSvc,
		Settings: settingsSvc,
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{},
		Media:    &contractMediaService{},
		Modules:  modulesSvc,
		Streams:  streamSvc,
	})
	r.POST("/torrents", torrents)

	req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{"action":"add","link":"magnet:?xt=urn:btih:0102030405060708090a0b0c0d0e0f1011121314","save_to_db":true,"data":"payload"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if streamSvc.parseLinkCalls != 1 {
		t.Fatalf("expected StreamService.ParseLink to be called once, got %d", streamSvc.parseLinkCalls)
	}

	if torrentsSvc.addCalls != 1 {
		t.Fatalf("expected TorrentService.Add to be called once, got %d", torrentsSvc.addCalls)
	}

	if torrentsSvc.addTitle != "parsed-title" || torrentsSvc.addPoster != "parsed-poster" || torrentsSvc.addCategory != "parsed-category" {
		t.Fatalf("expected parsed metadata, got title=%q poster=%q category=%q", torrentsSvc.addTitle, torrentsSvc.addPoster, torrentsSvc.addCategory)
	}

	if torrentsSvc.addData != "payload" {
		t.Fatalf("expected request data to be preserved, got %q", torrentsSvc.addData)
	}

	if !modulesSvc.restartCalled {
		t.Fatal("expected DLNA restart to be requested through ModulesService")
	}
}

func TestTorrentsAddMapsStreamParseError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &testTorrentService{}
	streamSvc := &testStreamService{parseLinkErr: contracts.ErrStreamInvalidTorrsHash}

	r := gin.New()
	withServices(t, r, &contracts.APIServices{
		Torrents: torrentsSvc,
		Settings: &contractSettingsService{},
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{},
		Media:    &contractMediaService{},
		Modules:  &contractModulesService{},
		Streams:  streamSvc,
	})
	r.POST("/torrents", torrents)

	req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{"action":"add","link":"torrs://bad"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "invalid torrs hash") {
		t.Fatalf("expected torrs hash validation message, got %s", w.Body.String())
	}

	if torrentsSvc.addCalls != 0 {
		t.Fatalf("TorrentService.Add should not be called after parse error, got %d", torrentsSvc.addCalls)
	}
}
