package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"server/torr/state"
)

type playbackStub struct {
	allRes      PlaylistPayload
	playlistRes PlaylistPayload
	playlistErr error
	resolveErr  error
	m3uResult   string
}

func (s playbackStub) BuildAllPlaylist(host string, torrents TorrentService) PlaylistPayload {
	return s.allRes
}

func (s playbackStub) BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents TorrentService, viewed ViewedService) (PlaylistPayload, error) {
	return s.playlistRes, s.playlistErr
}

func (s playbackStub) BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed ViewedService) string {
	return s.m3uResult
}

func (s playbackStub) ResolvePlay(hash, index string, unauthorized bool, torrents TorrentService) (PlayTarget, error) {
	return PlayTarget{}, s.resolveErr
}

func TestPlayMapsUnauthorizedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setTestServices(t, &APIServices{
		Playback: playbackStub{resolveErr: ErrPlayUnauthorized},
	})

	r := gin.New()
	r.GET("/play/:hash/:id", play)

	req := httptest.NewRequest(http.MethodGet, "/play/hash/1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic") {
		t.Fatalf("expected WWW-Authenticate header, got %q", got)
	}
}

func TestPlayListMapsNotFoundError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setTestServices(t, &APIServices{
		Playback: playbackStub{playlistErr: ErrPlaylistTorrentNotFound},
	})

	r := gin.New()
	r.GET("/playlist/*fname", playList)

	req := httptest.NewRequest(http.MethodGet, "/playlist/list.m3u?hash=deadbeef", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestAllPlayListUsesPlaybackServiceResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setTestServices(t, &APIServices{
		Playback: playbackStub{
			allRes: PlaylistPayload{
				Name: "all.m3u",
				Hash: "abc123",
				Body: "#EXTM3U\n#EXTINF:0,Demo\nhttp://localhost/stream/demo\n",
			},
		},
	})

	r := gin.New()
	r.GET("/playlistall/all.m3u", allPlayList)

	req := httptest.NewRequest(http.MethodGet, "/playlistall/all.m3u", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "audio/x-mpegurl") {
		t.Fatalf("expected m3u content-type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "#EXTM3U") {
		t.Fatalf("expected m3u body, got %s", w.Body.String())
	}
}

func setTestServices(t *testing.T, s *APIServices) {
	t.Helper()
	apiServicesMu.Lock()
	prev := apiServices
	apiServicesMu.Unlock()

	SetServices(s)
	t.Cleanup(func() {
		apiServicesMu.Lock()
		apiServices = prev
		apiServicesMu.Unlock()
	})
}
