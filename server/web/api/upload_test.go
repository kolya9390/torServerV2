package api

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"server/internal/app/contracts"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/gin-gonic/gin"

	"server/torr/state"
)

type uploadTorrentService struct {
	mockTorrentService

	addCalled      bool
	finalizeCalled bool
}

func (s *uploadTorrentService) Add(spec contracts.TorrentSpec, title, poster, data, category string) (contracts.TorrentHandle, error) {
	if spec.HashHex() == "" {
		return nil, errors.New("torrent spec is nil")
	}

	s.addCalled = true

	return nil, nil
}

func (s *uploadTorrentService) Status(contracts.TorrentHandle) *state.TorrentStatus {
	return &state.TorrentStatus{
		Title: "uploaded",
		Hash:  "0123456789abcdef0123456789abcdef01234567",
		Stat:  state.TorrentAdded,
	}
}

func (s *uploadTorrentService) EnqueueMetadataFinalize(contracts.TorrentHandle, *contracts.TorrentSpec, bool) bool {
	s.finalizeCalled = true

	return true
}

type uploadStreamService struct{ contracts.StreamService }

func (s uploadStreamService) ParseTorrentFile(reader io.Reader) (contracts.TorrentSpec, error) {
	if _, err := io.ReadAll(reader); err != nil {
		return contracts.TorrentSpec{}, err
	}

	return contracts.NewTorrentSpec("0123456789abcdef0123456789abcdef01234567", nil), nil
}

func TestTorrentUploadRejectsOversizedMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &uploadTorrentService{}

	body, contentType := multipartBody(t, "file", "large.torrent", io.LimitReader(zeroReader{}, maxTorrentUploadBodyBytes+1024))

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  uploadStreamService{},
	})))
	r.POST("/torrent/upload", torrentUpload)

	req := httptest.NewRequest(http.MethodPost, "/torrent/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", w.Code, w.Body.String())
	}

	if torrentsSvc.addCalled {
		t.Fatal("oversized upload should be rejected before torrent service is called")
	}
}

func TestTorrentUploadAcceptsNormalTorrentFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &uploadTorrentService{}

	body, contentType := multipartBody(t, "file", "small.torrent", bytes.NewReader(validTorrentBytes(t)))

	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: torrentsSvc,
		Streams:  uploadStreamService{},
	})))
	r.POST("/torrent/upload", torrentUpload)

	req := httptest.NewRequest(http.MethodPost, "/torrent/upload", body)
	req.Header.Set("Content-Type", contentType)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if !torrentsSvc.addCalled {
		t.Fatal("expected torrent service Add to be called")
	}

	if !torrentsSvc.finalizeCalled {
		t.Fatal("expected metadata finalization to be queued")
	}
}

func multipartBody(t *testing.T, fieldName, fileName string, src io.Reader) (*bytes.Buffer, string) {
	t.Helper()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	if _, err := io.Copy(part, src); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	return body, writer.FormDataContentType()
}

func validTorrentBytes(t *testing.T) []byte {
	t.Helper()

	infoBytes, err := bencode.Marshal(metainfo.Info{
		PieceLength: 16 * 1024,
		Pieces:      make([]byte, 20),
		Name:        "empty.txt",
		Length:      0,
	})
	if err != nil {
		t.Fatalf("marshal torrent info: %v", err)
	}

	meta := metainfo.MetaInfo{
		Announce:  "http://tracker.invalid/announce",
		InfoBytes: infoBytes,
	}

	var buf bytes.Buffer
	if err := meta.Write(&buf); err != nil {
		t.Fatalf("write torrent metainfo: %v", err)
	}

	return buf.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)

	return len(p), nil
}
