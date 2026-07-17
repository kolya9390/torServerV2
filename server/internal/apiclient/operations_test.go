package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTypedOperationsBuildExpectedRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		response   string
		headerName string
		header     string
		invoke     func(context.Context, *Client) error
	}{
		{
			name:     "list torrents",
			method:   http.MethodPost,
			path:     torrentsPath,
			body:     `{"action":"list"}`,
			response: `[]`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListTorrents(ctx)

				return err
			},
		},
		{
			name:     "get torrent",
			method:   http.MethodPost,
			path:     torrentsPath,
			body:     `{"action":"get","hash":"abc"}`,
			response: `{}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetTorrent(ctx, "abc")

				return err
			},
		},
		{
			name:     "add torrent",
			method:   http.MethodPost,
			path:     torrentsPath,
			body:     `{"action":"add","link":"magnet:test","title":"Movie","save_to_db":true}`,
			response: `{}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.AddTorrent(ctx, AddTorrentRequest{Link: "magnet:test", Title: "Movie", Save: true})

				return err
			},
		},
		{
			name:   "remove torrent",
			method: http.MethodPost,
			path:   torrentsPath,
			body:   `{"action":"rem","hash":"abc"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.RemoveTorrent(ctx, "abc")
			},
		},
		{
			name:   "drop torrent",
			method: http.MethodPost,
			path:   torrentsPath,
			body:   `{"action":"drop","hash":"abc"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.DropTorrent(ctx, "abc")
			},
		},
		{
			name:   "wipe torrents",
			method: http.MethodPost,
			path:   torrentsPath,
			body:   `{"action":"wipe"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.WipeTorrents(ctx)
			},
		},
		{
			name:     "get settings",
			method:   http.MethodPost,
			path:     settingsPath,
			body:     `{"action":"get"}`,
			response: `{}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetSettings(ctx)

				return err
			},
		},
		{
			name:   "set settings",
			method: http.MethodPost,
			path:   settingsPath,
			body:   `{"action":"set","sets":{"CacheSize":67108864}}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.SetSettings(ctx, SettingsPatch{"CacheSize": int64(64 << 20)})
			},
		},
		{
			name:   "reset settings",
			method: http.MethodPost,
			path:   settingsPath,
			body:   `{"action":"def"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.ResetSettings(ctx)
			},
		},
		{
			name:     "list users",
			method:   http.MethodGet,
			path:     authUsersPath,
			response: `{}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListUsers(ctx)

				return err
			},
		},
		{
			name:   "add user",
			method: http.MethodPost,
			path:   authUsersPath,
			body:   `{"username":"admin","password":"test-password"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.AddUser(ctx, "admin", "test-password")
			},
		},
		{
			name:   "remove user",
			method: http.MethodDelete,
			path:   authUsersPath + "/alice%20smith",
			invoke: func(ctx context.Context, client *Client) error {
				return client.RemoveUser(ctx, "alice smith")
			},
		},
		{
			name:     "shutdown token status",
			method:   http.MethodGet,
			path:     shutdownTokenPath,
			response: `{"configured":true}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ShutdownTokenStatus(ctx)

				return err
			},
		},
		{
			name:     "generate shutdown token",
			method:   http.MethodPost,
			path:     shutdownTokenGenerateURL,
			response: `{"status":"generated","token":"generated-token"}`,
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GenerateShutdownToken(ctx)

				return err
			},
		},
		{
			name:   "set shutdown token",
			method: http.MethodPost,
			path:   shutdownTokenPath,
			body:   `{"token":"configured-token"}`,
			invoke: func(ctx context.Context, client *Client) error {
				return client.SetShutdownToken(ctx, "configured-token")
			},
		},
		{
			name:       "public shutdown",
			method:     http.MethodPost,
			path:       "/api/v1/shutdown/user%20request",
			headerName: "X-TS-Shutdown-Token",
			header:     "configured-token",
			invoke: func(ctx context.Context, client *Client) error {
				return client.Shutdown(ctx, ShutdownRequest{
					Mode:   "public",
					Reason: "user request",
					Token:  "configured-token",
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method {
					t.Errorf("method = %s, want %s", request.Method, test.method)
				}

				if request.URL.EscapedPath() != test.path {
					t.Errorf("path = %q, want %q", request.URL.EscapedPath(), test.path)
				}

				if test.headerName != "" && request.Header.Get(test.headerName) != test.header {
					t.Errorf("header %s = %q, want %q", test.headerName, request.Header.Get(test.headerName), test.header)
				}

				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				}

				assertJSONBody(t, body, test.body)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, test.response)
			}))
			t.Cleanup(server.Close)

			client := mustNewClient(t, Options{BaseURL: server.URL})
			if err := test.invoke(context.Background(), client); err != nil {
				t.Fatalf("operation: %v", err)
			}
		})
	}
}

func TestTypedOperationsDecodeResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		switch request.URL.Path {
		case "/api/v1/version":
			_, _ = io.WriteString(writer, compatibleVersionResponse)
		case "/readyz":
			_, _ = io.WriteString(writer, `{"status":"ready","http":true,"torrent":true}`)
		case torrentsPath:
			_, _ = io.WriteString(writer, `[{"title":"Movie","hash":"abc","file_stats":[{"id":2,"path":"movie.mkv","length":42}]}]`)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, Options{BaseURL: server.URL})
	version, err := client.Version(context.Background())
	if err != nil || version.Current != "v1" || version.Product != expectedProduct {
		t.Fatalf("Version() = %+v, %v", version, err)
	}

	readiness, err := client.Readiness(context.Background())
	if err != nil || readiness.Status != "ready" || !readiness.HTTP || !readiness.Torrent {
		t.Fatalf("Readiness() = %+v, %v", readiness, err)
	}

	torrents, err := client.ListTorrents(context.Background())
	if err != nil || len(torrents) != 1 || len(torrents[0].FileStats) != 1 || torrents[0].FileStats[0].ID != 2 {
		t.Fatalf("ListTorrents() = %+v, %v", torrents, err)
	}
}

func TestUploadTorrentBuildsMultipartRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != torrentUploadPath {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}

		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			http.Error(writer, "invalid multipart request", http.StatusBadRequest)

			return
		}

		if got := request.FormValue("title"); got != "Movie" {
			t.Errorf("title = %q", got)
		}

		if got := request.FormValue("save"); got != "true" {
			t.Errorf("save = %q", got)
		}

		file, header, err := request.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			http.Error(writer, "missing multipart file", http.StatusBadRequest)

			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("read form file: %v", err)
			http.Error(writer, "read multipart file", http.StatusInternalServerError)

			return
		}

		if header.Filename != "movie.torrent" || string(content) != "torrent metadata" {
			t.Errorf("upload = %q %q", header.Filename, content)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"title":"Movie","hash":"abc"}`)
	}))
	t.Cleanup(server.Close)

	filePath := filepath.Join(t.TempDir(), "movie.torrent")
	if err := os.WriteFile(filePath, []byte("torrent metadata"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	client := mustNewClient(t, Options{BaseURL: server.URL})
	response, err := client.UploadTorrent(context.Background(), UploadTorrentRequest{
		FilePath: filePath,
		Title:    "Movie",
		Save:     true,
	})
	if err != nil {
		t.Fatalf("UploadTorrent(): %v", err)
	}

	if response.Title != "Movie" || response.Hash != "abc" {
		t.Fatalf("response = %+v", response)
	}
}

func TestSettingsValuesReturnsIndependentCopy(t *testing.T) {
	t.Parallel()

	var settings Settings
	if err := json.Unmarshal([]byte(`{"CacheSize":67108864,"Profiles":[{"name":"default"}]}`), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}

	first := settings.Values()
	first["CacheSize"] = float64(1)
	profiles := first["Profiles"].([]any)
	profiles[0].(map[string]any)["name"] = "mutated"

	second := settings.Values()
	if second["CacheSize"] != float64(67108864) {
		t.Fatalf("settings mutated through Values: %#v", second)
	}

	secondProfiles := second["Profiles"].([]any)
	if secondProfiles[0].(map[string]any)["name"] != "default" {
		t.Fatalf("nested settings mutated through Values: %#v", second)
	}
}

func assertJSONBody(t *testing.T, got []byte, want string) {
	t.Helper()

	if want == "" {
		if len(strings.TrimSpace(string(got))) != 0 {
			t.Errorf("body = %s, want empty", got)
		}

		return
	}

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode actual JSON %q: %v", got, err)
	}

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected JSON %q: %v", want, err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON body = %#v, want %#v", gotValue, wantValue)
	}
}
