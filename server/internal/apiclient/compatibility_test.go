package apiclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const compatibleVersionResponse = `{"product":"torrserver","application_version":"v1.0.0-beta.3",` +
	`"current":"v1","capabilities":["management-api-v1"],"deprecated":["legacy-root"]}`

func TestVersionCompatibilityAcceptsApplicationBuildDifferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		response           string
		applicationVersion string
		legacy             bool
	}{
		{
			name:               "equal prerelease",
			response:           compatibleVersionResponse,
			applicationVersion: "v1.0.0-beta.3",
		},
		{
			name: "previous legacy beta contract",
			response: `{"current":"v1","deprecated":["legacy-root"],` +
				`"deprecation":"2025-06-01T00:00:00Z","sunset":"Tue, 30 Jun 2026 23:59:59 GMT"}`,
			applicationVersion: unknownApplicationVersion,
			legacy:             true,
		},
		{
			name: "older compatible prerelease",
			response: `{"product":"torrserver","application_version":"v1.0.0-beta.1",` +
				`"current":"v1","capabilities":["management-api-v1"]}`,
			applicationVersion: "v1.0.0-beta.1",
		},
		{
			name: "newer compatible application with unknown extensions",
			response: `{"product":"torrserver","application_version":"v1.3.0",` +
				`"current":"v1","capabilities":["management-api-v1","future-optional"],` +
				`"torrent_engine":{"version":"v99.0.0"},"future":{"mode":"supported"}}`,
			applicationVersion: "v1.3.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := newVersionTestClient(t, http.StatusOK, test.response)
			version, err := client.Version(context.Background())
			if err != nil {
				t.Fatalf("Version(): %v", err)
			}

			if version.ApplicationVersion != test.applicationVersion || version.Current != supportedAPIVersion {
				t.Fatalf("version = %+v", version)
			}

			if version.LegacyContract != test.legacy {
				t.Fatalf("legacy contract = %t, want %t", version.LegacyContract, test.legacy)
			}
		})
	}
}

func TestVersionCompatibilityRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		response   string
		wantKind   CompatibilityErrorKind
		wantText   string
	}{
		{
			name:       "missing version endpoint",
			statusCode: http.StatusNotFound,
			response:   `{"error":"not found"}`,
			wantKind:   CompatibilityEndpointMissing,
			wantText:   "may be outdated",
		},
		{
			name:       "authentication failure",
			statusCode: http.StatusUnauthorized,
			response:   `{"error":"authentication required"}`,
			wantKind:   CompatibilityAuthentication,
			wantText:   "verify --user and TS_PASSWORD",
		},
		{
			name:       "non TorrServer endpoint",
			statusCode: http.StatusOK,
			response: `{"product":"different-service","application_version":"v1.0.0",` +
				`"current":"v1","capabilities":["management-api-v1"]}`,
			wantKind: CompatibilityWrongProduct,
			wantText: "not TorrServer",
		},
		{
			name:       "missing product marker",
			statusCode: http.StatusOK,
			response:   `{"current":"v1"}`,
			wantKind:   CompatibilityWrongProduct,
			wantText:   "product marker is missing",
		},
		{
			name:       "unsupported API",
			statusCode: http.StatusOK,
			response: `{"product":"torrserver","application_version":"v2.0.0",` +
				`"current":"v2","capabilities":["management-api-v2"]}`,
			wantKind: CompatibilityUnsupportedAPI,
			wantText: "supports \"v1\"",
		},
		{
			name:       "malformed capabilities type",
			statusCode: http.StatusOK,
			response: `{"product":"torrserver","application_version":"v1.0.0",` +
				`"current":"v1","capabilities":"management-api-v1"}`,
			wantKind: CompatibilityMalformed,
			wantText: "malformed",
		},
		{
			name:       "required capability missing",
			statusCode: http.StatusOK,
			response: `{"product":"torrserver","application_version":"v1.0.0",` +
				`"current":"v1","capabilities":["future-optional"]}`,
			wantKind: CompatibilityMalformed,
			wantText: requiredManagementCapability,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			client := newVersionTestClient(t, test.statusCode, test.response)
			_, err := client.Version(context.Background())

			var compatibilityErr *CompatibilityError
			if !errors.As(err, &compatibilityErr) {
				t.Fatalf("error = %v, want CompatibilityError", err)
			}

			if compatibilityErr.Kind != test.wantKind || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, kind = %q", err, compatibilityErr.Kind)
			}
		})
	}
}

func TestVersionCompatibilityClassifiesUnreachableServer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	client := mustNewClient(t, Options{BaseURL: serverURL, Timeout: time.Second})
	_, err := client.Version(context.Background())

	var compatibilityErr *CompatibilityError
	if !errors.As(err, &compatibilityErr) || compatibilityErr.Kind != CompatibilityTransport {
		t.Fatalf("error = %v, want transport CompatibilityError", err)
	}
}

func newVersionTestClient(t *testing.T, statusCode int, response string) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != versionPath {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(statusCode)
		if _, err := io.WriteString(writer, response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return mustNewClient(t, Options{BaseURL: server.URL})
}
