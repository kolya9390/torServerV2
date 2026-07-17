package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNoDirectSettingsArgsUsage(t *testing.T) {
	goFiles := collectGoFiles(t, projectRoot(t), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		ast.Inspect(f, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Name == "settings" && sel.Sel.Name == "Args" {
				t.Errorf("forbidden direct settings.Args usage in %s", path)
			}

			return true
		})
	}
}

func TestAPILayerDoesNotImportInfraDirectly(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "web", "api"), func(path string) bool {
		if strings.HasSuffix(path, "_test.go") {
			return false
		}

		if strings.HasSuffix(path, "services.go") {
			return false
		}

		if strings.Contains(path, string(filepath.Separator)+"utils"+string(filepath.Separator)) {
			return false
		}

		return true
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg := importPath(t, path, imp)
			if isForbiddenTransportInfraImport(pkg) {
				t.Errorf("forbidden import %q in transport file %s", pkg, path)
			}
		}
	}
}

func TestAPIHandlersDoNotUseBroadAPIServices(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "web", "api"), func(path string) bool {
		if strings.HasSuffix(path, "_test.go") {
			return false
		}

		base := filepath.Base(path)

		return base != "route.go" && base != "services_registry.go"
	})

	for _, path := range goFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		source := string(content)
		if strings.Contains(source, "servicesFromContext(") {
			t.Errorf("handler must use narrow dependency groups instead of servicesFromContext in %s", path)
		}

		if strings.Contains(source, "*contracts.APIServices") {
			t.Errorf("handler must not accept broad *contracts.APIServices in %s", path)
		}
	}
}

func TestAPIHandlersUseNarrowTorrentContracts(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "web", "api"), func(path string) bool {
		if strings.HasSuffix(path, "_test.go") {
			return false
		}

		base := filepath.Base(path)

		return base != "route.go"
	})

	for _, path := range goFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		if strings.Contains(string(content), "contracts.TorrentService") {
			t.Errorf("API handlers must use consumer-driven torrent contracts instead of contracts.TorrentService in %s", path)
		}
	}
}

func TestStreamAndPlaybackServicesUseNarrowTorrentContracts(t *testing.T) {
	for _, relPath := range []string{
		filepath.Join("internal", "app", "apiservices", "stream_service.go"),
		filepath.Join("internal", "app", "apiservices", "playback.go"),
	} {
		path := filepath.Join(projectRoot(t), relPath)

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		if strings.Contains(string(content), "contracts.TorrentService") {
			t.Errorf("service orchestration must accept narrow torrent contracts instead of contracts.TorrentService in %s", path)
		}
	}
}

func TestTorrentSpecDoesNotExposeUntypedNativePayload(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	source := string(content)
	for _, forbidden := range []string{
		"native  any",
		"native any",
		"Native() any",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("TorrentSpec must use a typed adapter payload instead of %q", forbidden)
		}
	}
}

func TestAPIHandlersDoNotDependOnComposedStreamService(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "web", "api"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		if strings.Contains(string(content), "contracts.StreamService") {
			t.Errorf("API handlers must depend on parser/helper/orchestrator stream interfaces instead of contracts.StreamService in %s", path)
		}
	}
}

func TestLegacyStreamEndpointUsesCompatibilityAdapter(t *testing.T) {
	path := filepath.Join(projectRoot(t), "web", "api", "stream_legacy.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	source := string(content)
	if !strings.Contains(source, "newLegacyStreamAdapter(") || !strings.Contains(source, ".Handle(c)") {
		t.Fatalf("legacy stream endpoint must delegate to the compatibility adapter")
	}

	for _, forbidden := range []string{
		`GetQuery("preload")`,
		`GetQuery("stat")`,
		`GetQuery("save")`,
		`GetQuery("m3u")`,
		`GetQuery("play")`,
		"EnsureTorrent(",
		"SaveToDB(",
		"EnqueuePreload(",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("legacy stream parsing/orchestration must stay out of stream_legacy.go, found %q", forbidden)
		}
	}
}

func TestRouteRegistrationKeepsCompatibilityBoundariesExplicit(t *testing.T) {
	path := filepath.Join(projectRoot(t), "web", "api", "route.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	source := string(content)
	for _, required := range []string{
		"registerPreferredStreamRoutes(route, authorized)",
		"registerCompatibilityPlaybackRoutes(route, authorized)",
		"registerSearchRoutes(route, authorized, authCfg)",
		"Compatibility stream/playback routes",
		"Preferred stream API",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("route registration must keep preferred and compatibility boundaries explicit; missing %q", required)
		}
	}
}

func TestTransportAndAppDoNotCallGlobalRuntimeStateDirectly(t *testing.T) {
	for _, relDir := range []string{
		filepath.Join("internal", "app"),
		"web",
	} {
		goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), relDir), func(path string) bool {
			return !strings.HasSuffix(path, "_test.go")
		})

		for _, path := range goFiles {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			source := string(content)
			if strings.Contains(source, "settings.GetRuntimeState") {
				t.Errorf("use injected runtime-state provider instead of settings.GetRuntimeState in %s", path)
			}

			if strings.Contains(source, "settings.UpdateRuntimeState") {
				t.Errorf("use injected runtime-state updater instead of settings.UpdateRuntimeState in %s", path)
			}
		}
	}
}

func TestAppContractsDoNotImportTransportPackages(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "internal", "app", "contracts"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg := importPath(t, path, imp)
			if pkg == "server/web" || strings.HasPrefix(pkg, "server/web/") {
				t.Errorf("forbidden transport import %q in app contract file %s", pkg, path)
			}
		}
	}
}

func TestAppContractsDoNotExposeSearchInfrastructureDTOs(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		if pkg == "server/torznab" || strings.HasPrefix(pkg, "server/torznab/") {
			t.Errorf("app contracts must expose application-owned search DTOs instead of %q in %s", pkg, path)
		}
	}
}

func TestAppContractsDoNotExposeMediaProbeInfrastructureDTOs(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		if pkg == "gopkg.in/vansante/go-ffprobe.v2" {
			t.Errorf("app contracts must expose application-owned media probe DTOs instead of %q in %s", pkg, path)
		}
	}
}

func TestAppContractsDoNotExposeViewedSettingsDTOs(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	source := string(content)
	for _, forbidden := range []string{
		"*sets.Viewed",
		"[]*sets.Viewed",
		"*settings.Viewed",
		"[]*settings.Viewed",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("app contracts must expose application-owned viewed DTOs instead of %q in %s", forbidden, path)
		}
	}
}

func TestAppContractsDoNotImportSettingsPackage(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		if pkg == "server/settings" || strings.HasPrefix(pkg, "server/settings/") {
			t.Errorf("app contracts must expose application-owned settings DTOs instead of importing %q in %s", pkg, path)
		}
	}
}

func TestAppContractsDoNotImportTorrentStatePackage(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "contracts", "contracts.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		if pkg == "server/torr/state" || strings.HasPrefix(pkg, "server/torr/state/") {
			t.Errorf("app contracts must expose application-owned torrent status DTOs instead of importing %q in %s", pkg, path)
		}
	}
}

func TestTorrentsHandlerDoesNotImportParsingAdapters(t *testing.T) {
	path := filepath.Join(projectRoot(t), "web", "api", "torrents.go")
	f := parseFile(t, path)

	for _, imp := range f.Imports {
		pkg := importPath(t, path, imp)
		switch pkg {
		case "github.com/anacrolix/torrent", "server/torrshash", "server/web/api/utils":
			t.Errorf("forbidden parsing adapter import %q in torrents handler %s", pkg, path)
		}
	}
}

func TestTorrentsRequestBindingStaysOutOfHandler(t *testing.T) {
	handlerPath := filepath.Join(projectRoot(t), "web", "api", "torrents.go")
	bindingPath := filepath.Join(projectRoot(t), "web", "api", "torrents_binding.go")

	handler, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("read %s: %v", handlerPath, err)
	}

	binding, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}

	handlerSource := string(handler)
	if strings.Contains(handlerSource, "ShouldBindJSON") {
		t.Errorf("torrents handler should delegate request binding to torrents_binding.go")
	}

	if !strings.Contains(handlerSource, "bindTorrentRequest(c)") {
		t.Errorf("torrents handler should use the dedicated torrent request binder")
	}

	if !strings.Contains(string(binding), "ShouldBindJSON") {
		t.Errorf("torrent request binder should own JSON binding")
	}
}

func TestUploadRequestBindingStaysOutOfHandler(t *testing.T) {
	handlerPath := filepath.Join(projectRoot(t), "web", "api", "upload.go")
	bindingPath := filepath.Join(projectRoot(t), "web", "api", "upload_binding.go")

	handler, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("read %s: %v", handlerPath, err)
	}

	binding, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}

	handlerSource := string(handler)
	if strings.Contains(handlerSource, "MultipartForm") {
		t.Errorf("upload handler should delegate multipart binding to upload_binding.go")
	}

	if !strings.Contains(handlerSource, "bindTorrentUploadRequest(c)") {
		t.Errorf("upload handler should use the dedicated upload request binder")
	}

	if !strings.Contains(string(binding), "MultipartForm") {
		t.Errorf("upload request binder should own multipart binding")
	}
}

func TestCompactAPIRequestBindingStaysOutOfHandlers(t *testing.T) {
	cases := []struct {
		name         string
		handlerFile  string
		bindingFile  string
		forbidden    []string
		requiredCall string
		requiredBind string
	}{
		{
			name:         "settings",
			handlerFile:  "settings.go",
			bindingFile:  "settings_binding.go",
			forbidden:    []string{"ShouldBindJSON"},
			requiredCall: "bindSettingsRequest(c)",
			requiredBind: "ShouldBindJSON",
		},
		{
			name:         "storage",
			handlerFile:  "storage.go",
			bindingFile:  "storage_binding.go",
			forbidden:    []string{"ShouldBindJSON", "PostForm("},
			requiredCall: "bindStoragePreferencesRequest(c)",
			requiredBind: "ShouldBindJSON",
		},
		{
			name:         "torznab",
			handlerFile:  "torznab.go",
			bindingFile:  "torznab_binding.go",
			forbidden:    []string{"ShouldBindJSON", "QueryUnescape", "DefaultQuery("},
			requiredCall: "bindTorznabSearchRequest(c)",
			requiredBind: "ShouldBindJSON",
		},
		{
			name:         "cache",
			handlerFile:  "cache.go",
			bindingFile:  "cache_binding.go",
			forbidden:    []string{"ShouldBindJSON"},
			requiredCall: "bindCacheRequest(c)",
			requiredBind: "ShouldBindJSON",
		},
		{
			name:         "viewed",
			handlerFile:  "viewed.go",
			bindingFile:  "viewed_binding.go",
			forbidden:    []string{"ShouldBindJSON"},
			requiredCall: "bindViewedRequest(c)",
			requiredBind: "ShouldBindJSON",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handlerPath := filepath.Join(projectRoot(t), "web", "api", tc.handlerFile)
			bindingPath := filepath.Join(projectRoot(t), "web", "api", tc.bindingFile)

			handler, err := os.ReadFile(handlerPath)
			if err != nil {
				t.Fatalf("read %s: %v", handlerPath, err)
			}

			binding, err := os.ReadFile(bindingPath)
			if err != nil {
				t.Fatalf("read %s: %v", bindingPath, err)
			}

			handlerSource := string(handler)
			for _, forbidden := range tc.forbidden {
				if strings.Contains(handlerSource, forbidden) {
					t.Errorf("%s handler should delegate %q to %s", tc.name, forbidden, tc.bindingFile)
				}
			}

			if !strings.Contains(handlerSource, tc.requiredCall) {
				t.Errorf("%s handler should call %s", tc.name, tc.requiredCall)
			}

			if !strings.Contains(string(binding), tc.requiredBind) {
				t.Errorf("%s binding file should contain %s", tc.name, tc.requiredBind)
			}
		})
	}
}

func TestFocusedResponseMappingStaysOutOfHandlers(t *testing.T) {
	cases := []struct {
		name         string
		handlerFile  string
		responseFile string
		forbidden    []string
		requiredCall string
	}{
		{
			name:         "settings",
			handlerFile:  "settings.go",
			responseFile: "settings_response.go",
			forbidden:    []string{"json.Marshal", `Header("ETag"`},
			requiredCall: "writeSettingsResponse(c, deps.Settings.Current())",
		},
		{
			name:         "torznab",
			handlerFile:  "torznab.go",
			responseFile: "torznab_response.go",
			forbidden:    []string{"gin.H", "[]*contracts.SearchResult{}"},
			requiredCall: "writeTorznabSearchResponse(c, deps.Search.TorznabSearch(searchReq.Query, searchReq.Index))",
		},
		{
			name:         "torrents",
			handlerFile:  "torrents.go",
			responseFile: "torrents_response.go",
			forbidden:    []string{"c.JSON(200, deps.Queries.Status", "c.JSON(200, st)"},
			requiredCall: "writeTorrentStatusResponse(c, deps.Queries.Status(tor))",
		},
		{
			name:         "stream",
			handlerFile:  "stream_actions.go",
			responseFile: "stream_response.go",
			forbidden:    []string{`gin.H{"status": "saved"`},
			requiredCall: "writeStreamSaveResponse(c, tor.HashHex())",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handlerPath := filepath.Join(projectRoot(t), "web", "api", tc.handlerFile)
			responsePath := filepath.Join(projectRoot(t), "web", "api", tc.responseFile)

			handler, err := os.ReadFile(handlerPath)
			if err != nil {
				t.Fatalf("read %s: %v", handlerPath, err)
			}

			if _, err := os.Stat(responsePath); err != nil {
				t.Fatalf("response mapper file missing %s: %v", responsePath, err)
			}

			handlerSource := string(handler)
			for _, forbidden := range tc.forbidden {
				if strings.Contains(handlerSource, forbidden) {
					t.Errorf("%s handler should delegate response mapping instead of using %q", tc.name, forbidden)
				}
			}

			if !strings.Contains(handlerSource, tc.requiredCall) {
				t.Errorf("%s handler should call %s", tc.name, tc.requiredCall)
			}
		})
	}
}

func TestPreferredStreamRequestBindingStaysOutOfHandler(t *testing.T) {
	handlerPath := filepath.Join(projectRoot(t), "web", "api", "stream_actions.go")
	bindingPath := filepath.Join(projectRoot(t), "web", "api", "stream_binding.go")

	handler, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("read %s: %v", handlerPath, err)
	}

	binding, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatalf("read %s: %v", bindingPath, err)
	}

	handlerSource := string(handler)
	for _, forbidden := range []string{
		`c.Query("link")`,
		`c.Query("title")`,
		`c.Query("poster")`,
		`c.Query("category")`,
		`c.Query("index")`,
		`c.GetQuery("preload")`,
		`c.GetQuery("fromlast")`,
		`c.Param("fname")`,
	} {
		if strings.Contains(handlerSource, forbidden) {
			t.Errorf("preferred stream handler should delegate request binding instead of using %s", forbidden)
		}
	}

	for _, required := range []string{
		"bindStreamLinkRequest(c, deps.Parser)",
		"bindStreamM3URequest(c, deps.Parser)",
		"bindStreamPlayRequest(c, deps.Parser)",
		"bindStreamFileIndex(c, deps.Parser, tor.FileCount())",
	} {
		if !strings.Contains(handlerSource, required) {
			t.Errorf("preferred stream handler should call %s", required)
		}
	}

	bindingSource := string(binding)
	for _, required := range []string{
		`c.Query("link")`,
		`c.GetQuery("preload")`,
		`c.GetQuery("fromlast")`,
		`c.Param("fname")`,
	} {
		if !strings.Contains(bindingSource, required) {
			t.Errorf("stream binding file should own %s", required)
		}
	}
}

func TestOsExitOnlyInMain(t *testing.T) {
	root := projectRoot(t)
	goFiles := collectGoFiles(t, root, func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		ast.Inspect(f, func(node ast.Node) bool {
			sel, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if ident.Name != "os" || sel.Sel.Name != "Exit" {
				return true
			}

			cmdDir := filepath.Join(root, "cmd")
			cleanPath := filepath.Clean(path)
			isCommandMain := f.Name.Name == "main" && filepath.Base(cleanPath) == "main.go" &&
				strings.HasPrefix(cleanPath, filepath.Clean(cmdDir)+string(filepath.Separator))
			if !isCommandMain {
				t.Errorf("os.Exit is only allowed in cmd main entry points, found in %s", relativePath(t, root, path))
			}

			return true
		})
	}
}

func TestMainDoesNotOwnDaemonLifecycle(t *testing.T) {
	mainPath := filepath.Join(projectRoot(t), "cmd", "main.go")
	file := parseFile(t, mainPath)
	for _, imp := range file.Imports {
		pkg := importPath(t, mainPath, imp)
		switch pkg {
		case "server/bootstrap", "server/config", "server/internal/apiclient", "server/internal/cliapp",
			"server/log", "server/settings", "os/signal", "syscall", "github.com/spf13/cobra":
			t.Errorf("cmd/main.go must delegate daemon lifecycle instead of importing %q", pkg)
		}
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}

	if strings.Contains(string(content), "IsInvocation") {
		t.Error("cmd/main.go must not guess between daemon and CLI modes")
	}
}

func TestTorrctlMainIsThinCompositionRoot(t *testing.T) {
	mainPath := filepath.Join(projectRoot(t), "cmd", "torrctl", "main.go")
	file := parseFile(t, mainPath)
	allowed := map[string]struct{}{
		"context":                {},
		"os":                     {},
		"server/internal/cliapp": {},
	}

	for _, imp := range file.Imports {
		pkg := importPath(t, mainPath, imp)
		if _, ok := allowed[pkg]; !ok {
			t.Errorf("cmd/torrctl/main.go imports non-composition dependency %q", pkg)
		}
	}
}

func TestTorrserverMainIsThinCompositionRoot(t *testing.T) {
	mainPath := filepath.Join(projectRoot(t), "cmd", "torrserver", "main.go")
	file := parseFile(t, mainPath)
	allowed := map[string]struct{}{
		"context":                {},
		"fmt":                    {},
		"io":                     {},
		"os":                     {},
		"server/docs":            {},
		"server/internal/daemon": {},
	}

	for _, imp := range file.Imports {
		pkg := importPath(t, mainPath, imp)
		if _, ok := allowed[pkg]; !ok {
			t.Errorf("cmd/torrserver/main.go imports non-composition dependency %q", pkg)
		}
	}
}

func TestSettingsLayerDoesNotImportWebPackages(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "settings"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg := importPath(t, path, imp)

			if strings.HasPrefix(pkg, "server/web/") {
				t.Errorf("forbidden settings import %q in %s", pkg, path)
			}
		}
	}
}

func TestInternalAppDoesNotImportRootServerPackage(t *testing.T) {
	goFiles := collectGoFiles(t, filepath.Join(projectRoot(t), "internal", "app"), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	})

	for _, path := range goFiles {
		f := parseFile(t, path)
		for _, imp := range f.Imports {
			pkg := importPath(t, path, imp)

			if pkg == "server" {
				t.Errorf("forbidden internal/app import %q in %s", pkg, path)
			}
		}
	}
}

func TestRuntimeAPIServicesFactoryDoesNotCaptureGlobalProviders(t *testing.T) {
	path := filepath.Join(projectRoot(t), "internal", "app", "runtime_server.go")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	source := string(content)
	for _, forbidden := range []string{
		"SettingsProvider:  settings.DefaultSettingsProvider",
		"ArgsProvider:      settings.DefaultArgsProvider",
		"RuntimeState:      settings.DefaultRuntimeStateProvider",
		"runtimeState := settings.DefaultRuntimeStateProvider",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("default API service composition must use injected runtime deps instead of %q", forbidden)
		}
	}
}

func TestGlobalProviderCompatibilityCallsStayInAllowedFiles(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("bootstrap", "bootstrap.go"):                 true,
		filepath.Join("bootstrap", "cleanup.go"):                   true,
		filepath.Join("dlna", "dlna.go"):                           true,
		filepath.Join("internal", "app", "runtime_server.go"):      true,
		filepath.Join("internal", "startup", "checks.go"):          true,
		filepath.Join("settings", "args_provider.go"):              true,
		filepath.Join("settings", "provider.go"):                   true,
		filepath.Join("settings", "runtime_state.go"):              true,
		filepath.Join("settings", "settings.go"):                   true,
		filepath.Join("torr", "btserver.go"):                       true,
		filepath.Join("torr", "storage", "torrstor", "storage.go"): true,
		filepath.Join("torrfs", "torrfs.go"):                       true,
		filepath.Join("torrfs", "webdav", "webdav.go"):             true,
		filepath.Join("torznab", "torznab.go"):                     true,
		filepath.Join("web", "webinfra", "webinfra.go"):            true,
	}

	for _, path := range collectGoFiles(t, projectRoot(t), func(path string) bool {
		return !strings.HasSuffix(path, "_test.go")
	}) {
		rel, err := filepath.Rel(projectRoot(t), path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}

		if allowed[rel] {
			continue
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		source := string(content)
		for _, forbidden := range []string{
			"settings.DefaultSettingsProvider",
			"settings.DefaultArgsProvider",
			"settings.DefaultRuntimeStateProvider",
			"settings.GetRuntimeState(",
			"settings.UpdateRuntimeState(",
			"settings.SetArgs(",
			"torr.NewBTS(",
			"dlna.Start(",
			"torrfs.New(",
			"torznab.Search(",
			"torrstor.NewStorage(",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("global compatibility call %q must stay in approved composition/compatibility files, found in %s", forbidden, path)
			}
		}
	}
}

func importPath(t *testing.T, sourcePath string, imp *ast.ImportSpec) string {
	t.Helper()

	pkg, err := strconv.Unquote(imp.Path.Value)
	if err != nil {
		t.Fatalf("unquote import in %s: %v", sourcePath, err)
	}

	return pkg
}

func isForbiddenTransportInfraImport(pkg string) bool {
	if pkg == "github.com/anacrolix/torrent" || strings.HasPrefix(pkg, "github.com/anacrolix/torrent/") {
		return true
	}

	if pkg == "server/torr" || strings.HasPrefix(pkg, "server/torr/") {
		return true
	}

	return pkg == "server/modules" || pkg == "server/ffprobe"
}

func collectGoFiles(t *testing.T, root string, include func(string) bool) []string {
	t.Helper()

	files := make([]string, 0, 256)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		if include(path) {
			files = append(files, filepath.Clean(path))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk go files in %s: %v", root, err)
	}

	return files
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return file
}

func projectRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	root := filepath.Clean(filepath.Join(wd, "..", ".."))

	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil || info.IsDir() {
		t.Fatalf("cannot resolve project root from wd=%s", wd)
	}

	return root
}
