package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"server/internal/app/contracts"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	sets "server/settings"
	wauth "server/web/auth"
)

type contractTorrentService struct{ mockTorrentService }

type contractSettingsService struct {
	mockSettingsService
	current    *contracts.Settings
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

func (s *contractSettingsService) Current() *contracts.Settings {
	if s.current == nil {
		return &contracts.Settings{}
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

func (s *contractSearchService) TorznabSearch(query string, index int) []*contracts.SearchResult {
	return []*contracts.SearchResult{}
}

func (s *contractMediaService) ProbePlayURL(hash, fileID string) (contracts.MediaProbe, error) {
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
	if err := SetupRouteWithServices(r, func() sets.RuntimeState { return sets.RuntimeState{} }, services); err != nil {
		t.Fatalf("SetupRouteWithServices returned error: %v", err)
	}

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

	err := SetupRouteWithServices(gin.New(), func() sets.RuntimeState { return sets.RuntimeState{} }, &contracts.APIServices{})
	if err == nil {
		t.Fatal("expected SetupRouteWithServices to return incomplete services error")
	}

	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing dependency error, got %v", err)
	}
}

func TestServicesMiddlewareRequiresCompleteServices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, err := buildServicesMiddleware(&contracts.APIServices{})
	if err == nil {
		t.Fatal("expected buildServicesMiddleware to return incomplete services error")
	}

	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing dependency error, got %v", err)
	}
}

func TestHandlerMissingServicesContextFailsClosedWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/storage/settings", GetStorageSettings)

	req := httptest.NewRequest(http.MethodGet, "/storage/settings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}

	var body struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error response, got err: %v body=%s", err, w.Body.String())
	}

	if body.Error.Type != "internal_error" || body.Error.Message != "api services are not configured" {
		t.Fatalf("unexpected error response: %+v", body.Error)
	}
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

	if !torrentsSvc.finalizeCalled {
		t.Fatal("expected metadata finalization to be enqueued")
	}

	if torrentsSvc.finalizeSpec == nil {
		t.Fatal("expected new torrent path to pass parsed torrent spec to metadata finalization")
	}

	if !torrentsSvc.finalizeSave {
		t.Fatal("expected save_to_db=true to be passed to metadata finalization")
	}

	if !modulesSvc.restartCalled {
		t.Fatal("expected DLNA restart to be requested through ModulesService")
	}
}

func TestTorrentsAddExistingActiveFastPathEnqueuesSaveFinalize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const hash = "0102030405060708090a0b0c0d0e0f1011121314"

	existing := testTorrentHandle{
		status: &contracts.TorrentStatus{
			Hash:  hash,
			Title: "existing-title",
			Stat:  contracts.TorrentWorking,
		},
		hash:  hash,
		state: contracts.TorrentWorking,
		name:  "existing-title",
		files: 1,
	}
	torrentsSvc := &testTorrentService{getResult: existing}
	streamSvc := &testStreamService{}
	modulesSvc := &contractModulesService{}

	r := gin.New()
	withServices(t, r, &contracts.APIServices{
		Torrents: torrentsSvc,
		Settings: &contractSettingsService{enableDLNA: true},
		Viewed:   &contractViewedService{},
		System:   &contractSystemService{},
		Search:   &contractSearchService{},
		Media:    &contractMediaService{},
		Modules:  modulesSvc,
		Streams:  streamSvc,
	})
	r.POST("/torrents", torrents)

	req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{"action":"add","link":"magnet:?xt=urn:btih:`+hash+`","save_to_db":true}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if streamSvc.parseLinkCalls != 1 {
		t.Fatalf("expected StreamService.ParseLink to be called once, got %d", streamSvc.parseLinkCalls)
	}

	if torrentsSvc.addCalls != 0 {
		t.Fatalf("existing active torrent fast-path must not call Add, got %d calls", torrentsSvc.addCalls)
	}

	if !torrentsSvc.finalizeCalled {
		t.Fatal("expected save_to_db fast-path to enqueue metadata finalization")
	}

	if torrentsSvc.finalizeTor != existing {
		t.Fatalf("expected finalize to receive existing torrent handle, got %#v", torrentsSvc.finalizeTor)
	}

	if torrentsSvc.finalizeSpec != nil {
		t.Fatal("expected existing fast-path finalization to avoid replacing torrent spec")
	}

	if !torrentsSvc.finalizeSave {
		t.Fatal("expected save_to_db=true to be passed to fast-path finalization")
	}

	if modulesSvc.restartCalled {
		t.Fatal("existing active fast-path should return before DLNA restart")
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
