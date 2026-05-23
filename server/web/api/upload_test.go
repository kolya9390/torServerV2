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

func (s *uploadTorrentService) Status(contracts.TorrentHandle) *contracts.TorrentStatus {
	return &contracts.TorrentStatus{
		Title: "uploaded",
		Hash:  "0123456789abcdef0123456789abcdef01234567",
		Stat:  contracts.TorrentAdded,
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

func TestBindTorrentUploadRequestParsesFormFields(t *testing.T) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("save", "1"); err != nil {
		t.Fatalf("write save field: %v", err)
	}

	if err := writer.WriteField("title", "Example"); err != nil {
		t.Fatalf("write title field: %v", err)
	}

	if err := writer.WriteField("category", "Movies"); err != nil {
		t.Fatalf("write category field: %v", err)
	}

	if err := writer.WriteField("poster", "poster.jpg"); err != nil {
		t.Fatalf("write poster field: %v", err)
	}

	if err := writer.WriteField("data", "payload"); err != nil {
		t.Fatalf("write data field: %v", err)
	}

	file, err := writer.CreateFormFile("file", "small.torrent")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	if _, err := file.Write(validTorrentBytes(t)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	c := uploadBindingContext(t, body, writer.FormDataContentType())
	req, err := bindTorrentUploadRequest(c)
	if err != nil {
		t.Fatalf("expected upload request to bind, got %v", err)
	}
	defer req.Form.RemoveAll()

	if !req.Fields.Save {
		t.Fatalf("expected save field to bind")
	}

	if req.Fields.Title != "Example" {
		t.Fatalf("expected title Example, got %q", req.Fields.Title)
	}

	if req.Fields.Category != "Movies" {
		t.Fatalf("expected category Movies, got %q", req.Fields.Category)
	}

	if req.Fields.Poster != "poster.jpg" {
		t.Fatalf("expected poster poster.jpg, got %q", req.Fields.Poster)
	}

	if req.Fields.Data != "payload" {
		t.Fatalf("expected data payload, got %q", req.Fields.Data)
	}
}

func TestBindTorrentUploadRequestRequiresFile(t *testing.T) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("title", "Example"); err != nil {
		t.Fatalf("write title field: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	c := uploadBindingContext(t, body, writer.FormDataContentType())
	req, err := bindTorrentUploadRequest(c)
	if err == nil {
		t.Fatalf("expected validation error")
	}

	if req.Form != nil {
		defer req.Form.RemoveAll()
	}

	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.Field != "file" {
		t.Fatalf("expected file field, got %q", apiErr.Field)
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

func uploadBindingContext(t *testing.T, body *bytes.Buffer, contentType string) *gin.Context {
	t.Helper()

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/torrent/upload", body)
	c.Request.Header.Set("Content-Type", contentType)

	return c
}
