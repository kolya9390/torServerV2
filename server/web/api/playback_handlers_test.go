package api

import (
	"net/http"
	"net/http/httptest"
	"server/internal/app/contracts"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"server/torr/state"
)

type playbackStub struct {
	allRes      contracts.PlaylistPayload
	playlistRes contracts.PlaylistPayload
	playlistErr error
	resolveErr  error
	m3uResult   string
}

func (s playbackStub) BuildAllPlaylist(host string, torrents contracts.TorrentPlaylistService) contracts.PlaylistPayload {
	return s.allRes
}

func (s playbackStub) BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents contracts.TorrentPlaylistService, viewed contracts.ViewedService) (contracts.PlaylistPayload, error) {
	return s.playlistRes, s.playlistErr
}

func (s playbackStub) BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed contracts.ViewedService) string {
	return s.m3uResult
}

func (s playbackStub) ResolvePlay(hash, index string, unauthorized bool, torrents contracts.TorrentPlayService) (contracts.PlayTarget, error) {
	return contracts.PlayTarget{}, s.resolveErr
}

func TestPlayMapsUnauthorizedError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	setTestServices(t, r, &contracts.APIServices{
		Playback: playbackStub{resolveErr: contracts.ErrPlayUnauthorized},
	})
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

	r := gin.New()
	setTestServices(t, r, &contracts.APIServices{
		Playback: playbackStub{playlistErr: contracts.ErrPlaylistTorrentNotFound},
	})
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

	r := gin.New()
	setTestServices(t, r, &contracts.APIServices{
		Playback: playbackStub{
			allRes: contracts.PlaylistPayload{
				Name: "all.m3u",
				Hash: "abc123",
				Body: "#EXTM3U\n#EXTINF:0,Demo\nhttp://localhost/stream/demo\n",
			},
		},
	})
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

func setTestServices(t *testing.T, r *gin.Engine, s *contracts.APIServices) {
	t.Helper()

	r.Use(servicesMiddleware(newAPIServicesFixture(t, s)))
}
