package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"server/internal/app/contracts"

	"github.com/spf13/cobra"
)

func TestSettingsSetKeyValueSendsOnlyChangedField(t *testing.T) {
	t.Parallel()

	var requests []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)

			return
		}

		requests = append(requests, payload)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	if err := cmdSettingsSetKeyValue(client, globalOptions{Timeout: time.Second}, "ResponsiveMode", "false"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1 without a preliminary get", len(requests))
	}

	settings, ok := requests[0]["sets"].(map[string]any)
	if !ok {
		t.Fatalf("sets payload type = %T", requests[0]["sets"])
	}

	if len(settings) != 1 || settings["ResponsiveMode"] != false {
		t.Fatalf("settings patch = %#v, want only ResponsiveMode=false", settings)
	}
}

func TestSettingsPartialPatchesDoNotOverwriteConcurrentChanges(t *testing.T) {
	t.Parallel()

	current := map[string]any{
		"ConnectionsLimit": float64(25),
		"ResponsiveMode":   true,
	}

	var mutex sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Action string         `json:"action"`
			Sets   map[string]any `json:"sets"`
		}

		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)

			return
		}

		if payload.Action != "set" {
			t.Errorf("action = %q, want set", payload.Action)

			return
		}

		mutex.Lock()
		for key, value := range payload.Sets {
			current[key] = value
		}
		mutex.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	errors := make(chan error, 2)

	go func() {
		errors <- cmdSettingsSetKeyValue(client, globalOptions{Timeout: time.Second}, "ConnectionsLimit", "80")
	}()

	go func() {
		errors <- cmdSettingsSetKeyValue(client, globalOptions{Timeout: time.Second}, "ResponsiveMode", "false")
	}()

	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("concurrent settings update: %v", err)
		}
	}

	mutex.Lock()
	defer mutex.Unlock()

	if current["ConnectionsLimit"] != float64(80) || current["ResponsiveMode"] != false {
		t.Fatalf("merged settings = %#v", current)
	}
}

func TestKnownSettingsFieldsCoverSettingsContract(t *testing.T) {
	t.Parallel()

	known := make(map[string]settingsField, len(knownSettingsFields))
	for _, field := range knownSettingsFields {
		if _, exists := known[field.Key]; exists {
			t.Fatalf("duplicate setting definition %q", field.Key)
		}

		known[field.Key] = field
	}

	contractType := reflect.TypeFor[contracts.Settings]()
	for index := range contractType.NumField() {
		name := contractType.Field(index).Name
		if _, ok := known[name]; !ok {
			t.Errorf("settings contract field %s has no CLI definition", name)
		}

		delete(known, name)
	}

	for name := range known {
		t.Errorf("CLI definition %s is absent from settings contract", name)
	}
}

func TestParseSettingValueUsesFieldSpecificUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		value   string
		want    any
		wantErr bool
	}{
		{name: "byte size", key: "CacheSize", value: "1.5GB", want: int64(1610612736)},
		{name: "seconds duration", key: "TorrentDisconnectTimeout", value: "2m", want: 120},
		{name: "minutes duration", key: "WarmDiskCacheTTLMin", value: "2h", want: 120},
		{name: "milliseconds duration", key: "DiskSyncIntervalMS", value: "2s", want: 2000},
		{name: "plain integer", key: "ConnectionsLimit", value: "50", want: 50},
		{name: "size rejected for plain integer", key: "ConnectionsLimit", value: "50MB", wantErr: true},
		{name: "duration rejected for megabytes", key: "WarmDiskCacheSizeMB", value: "2m", wantErr: true},
		{name: "enum validation", key: "CoreProfile", value: "impossible", wantErr: true},
		{name: "range validation", key: "ReaderReadAHead", value: "101", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			field := findFieldByKey(test.key)
			if field == nil {
				t.Fatalf("field %s not found", test.key)
			}

			got, err := parseSettingValue(*field, test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseSettingValue(%q) unexpectedly succeeded: %#v", test.value, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseSettingValue(%q): %v", test.value, err)
			}

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseSettingValue(%q) = %#v, want %#v", test.value, got, test.want)
			}
		})
	}
}

func TestSettingsSetRejectsUnknownAndReadOnlyFields(t *testing.T) {
	t.Parallel()

	client := &apiClient{}
	tests := []struct {
		key      string
		wantText string
	}{
		{key: "NotASetting", wantText: "unknown setting"},
		{key: "EnableDebug", wantText: "config.yml"},
		{key: "StoreSettingsInJSON", wantText: "storage settings API"},
	}

	for _, test := range tests {
		err := cmdSettingsSetKeyValue(client, globalOptions{Timeout: time.Second}, test.key, "true")
		if err == nil || !strings.Contains(err.Error(), test.wantText) {
			t.Fatalf("set %s error = %v, want text %q", test.key, err, test.wantText)
		}
	}
}

func TestReadSettingsPayloadValidatesPartialPatch(t *testing.T) {
	t.Parallel()

	patch, err := readSettingsPayload(
		`{"connectionslimit":80,"WarmDiskCacheSizeMB":9007199254740993}`,
		"",
	)
	if err != nil {
		t.Fatalf("read valid patch: %v", err)
	}

	if len(patch) != 2 || patch["ConnectionsLimit"] != 80 {
		t.Fatalf("normalized patch = %#v", patch)
	}

	if patch["WarmDiskCacheSizeMB"] != int64(9007199254740993) {
		t.Fatalf("large int64 lost precision: %#v", patch["WarmDiskCacheSizeMB"])
	}

	if _, err := readSettingsPayload(`{"Unknown":1}`, ""); err == nil {
		t.Fatal("unknown JSON field unexpectedly accepted")
	}

	if _, err := readSettingsPayload(`{"EnableDebug":true}`, ""); err == nil || !strings.Contains(err.Error(), "config.yml") {
		t.Fatalf("EnableDebug JSON error = %v", err)
	}

	if _, err := readSettingsPayload(`{}`, ""); err == nil {
		t.Fatal("empty settings patch unexpectedly accepted")
	}

	if _, err := readSettingsPayload(`{"ConnectionsLimit":50} {}`, ""); err == nil {
		t.Fatal("multiple JSON documents unexpectedly accepted")
	}
}

func TestFormatSettingValueKeepsIntegersExact(t *testing.T) {
	t.Parallel()

	connections := findFieldByKey("ConnectionsLimit")
	if got := formatSettingValue(connections, float64(1000000000)); got != "1000000000" {
		t.Fatalf("integer formatting = %q", got)
	}

	cache := findFieldByKey("CacheSize")
	if got := formatSettingValue(cache, float64(128*1024*1024)); got != "128 MB" {
		t.Fatalf("cache formatting = %q", got)
	}
}

func TestFindFieldByKeyIncludesStartupPreloadPolicy(t *testing.T) {
	t.Parallel()

	field := findFieldByKey("startuppreloadpolicy")
	if field == nil {
		t.Fatal("StartupPreloadPolicy field not found")
	}

	if got, want := field.Kind, settingString; got != want {
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
