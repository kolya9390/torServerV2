package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindTorrentRequestParsesKnownPayload(t *testing.T) {
	c := torrentBindingContext(t, `{"action":"add","link":"magnet:?xt=urn:btih:abc","save_to_db":true}`)

	req, err := bindTorrentRequest(c)
	if err != nil {
		t.Fatalf("expected request to bind, got %v", err)
	}

	if req.Action != "add" {
		t.Fatalf("expected action add, got %q", req.Action)
	}

	if req.Link != "magnet:?xt=urn:btih:abc" {
		t.Fatalf("expected link to bind, got %q", req.Link)
	}

	if !req.SaveToDB {
		t.Fatalf("expected save_to_db to bind")
	}
}

func TestBindTorrentRequestRejectsInvalidJSON(t *testing.T) {
	c := torrentBindingContext(t, `{`)

	_, err := bindTorrentRequest(c)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.Field != "request" {
		t.Fatalf("expected request field, got %q", apiErr.Field)
	}
}

func TestBindTorrentRequestRequiresAction(t *testing.T) {
	c := torrentBindingContext(t, `{}`)

	_, err := bindTorrentRequest(c)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.Field != "action" {
		t.Fatalf("expected action field, got %q", apiErr.Field)
	}
}

func torrentBindingContext(t *testing.T, body string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	return c
}
