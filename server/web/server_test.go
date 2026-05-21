package web

import (
	"net/http"
	"testing"
)

func TestNewHTTPServerHasHeaderLimitsAndStreamingTimeouts(t *testing.T) {
	srv := newHTTPServer("127.0.0.1:0", http.NewServeMux())

	if srv.ReadHeaderTimeout != httpReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, httpReadHeaderTimeout)
	}

	if srv.MaxHeaderBytes != httpMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, httpMaxHeaderBytes)
	}

	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want 0 for streaming", srv.ReadTimeout)
	}

	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0 for streaming", srv.WriteTimeout)
	}

	if srv.IdleTimeout != httpIdleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", srv.IdleTimeout, httpIdleTimeout)
	}
}
