package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestSettingsSetKeyValuePreservesCurrentSettings(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"CacheSize":            float64(1073741824),
		"ConnectionsLimit":     float64(120),
		"ResponsiveMode":       true,
		"StartupPreloadPolicy": "legacy",
	}

	var setPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/settings" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch req["action"] {
		case "get":
			w.Header().Set("Content-Type", "application/json")

			if err := json.NewEncoder(w).Encode(current); err != nil {
				t.Fatalf("encode settings: %v", err)
			}
		case "set":
			sets, ok := req["sets"].(map[string]any)
			if !ok {
				t.Fatalf("sets payload type = %T, want map[string]any", req["sets"])
			}

			setPayload = sets

			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected action %v", req["action"])
		}
	}))
	defer server.Close()

	cli, err := newAPIClient(globalOptions{Server: server.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := cmdSettingsSetKeyValue(cli, globalOptions{Timeout: time.Second}, "ResponsiveMode", "false"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	if got, want := setPayload["ResponsiveMode"], false; got != want {
		t.Fatalf("ResponsiveMode = %v, want %v", got, want)
	}

	if got, want := setPayload["CacheSize"], current["CacheSize"]; got != want {
		t.Fatalf("CacheSize = %v, want preserved %v", got, want)
	}

	if got, want := setPayload["ConnectionsLimit"], current["ConnectionsLimit"]; got != want {
		t.Fatalf("ConnectionsLimit = %v, want preserved %v", got, want)
	}

	if got, want := setPayload["StartupPreloadPolicy"], current["StartupPreloadPolicy"]; got != want {
		t.Fatalf("StartupPreloadPolicy = %v, want preserved %v", got, want)
	}
}

func TestFindFieldByKeyIncludesStartupPreloadPolicy(t *testing.T) {
	t.Parallel()

	field := findFieldByKey("startuppreloadpolicy")
	if field == nil {
		t.Fatal("StartupPreloadPolicy field not found")
	}

	if got, want := field.Type, "string"; got != want {
		t.Fatalf("StartupPreloadPolicy type = %q, want %q", got, want)
	}
}

func TestValidateSettingsSetArgsAllowsJSONWithoutDummyArg(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().String("json", `{"CacheSize":1024}`, "")
	cmd.Flags().String("file", "", "")

	if err := validateSettingsSetArgs(cmd, nil); err != nil {
		t.Fatalf("json settings set without positional arg returned error: %v", err)
	}

	if err := validateSettingsSetArgs(cmd, []string{"_"}); err == nil {
		t.Fatal("json settings set with positional arg should fail")
	}
}
