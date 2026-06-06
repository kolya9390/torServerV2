package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"server/internal/app/contracts"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/gin-gonic/gin"
)

type uploadTorrentService struct {
	mockTorrentService

	addCalls      int
	finalizeCalls int
	statusCalls   int
	addErrors     map[int]error
}

func (s *uploadTorrentService) Add(
	spec contracts.TorrentSpec,
	title, poster, data, category string,
) (contracts.TorrentHandle, error) {
	if spec.HashHex() == "" {
		return nil, errors.New("torrent spec is nil")
	}

	s.addCalls++
	if err := s.addErrors[s.addCalls]; err != nil {
		return nil, err
	}

	return nil, nil
}

func (s *uploadTorrentService) Status(contracts.TorrentHandle) *contracts.TorrentStatus {
	s.statusCalls++

	return &contracts.TorrentStatus{
		Title: fmt.Sprintf("uploaded-%d", s.statusCalls),
		Hash:  fmt.Sprintf("%040d", s.statusCalls),
		Stat:  contracts.TorrentAdded,
	}
}

func (s *uploadTorrentService) EnqueueMetadataFinalize(contracts.TorrentHandle, *contracts.TorrentSpec, bool) bool {
	s.finalizeCalls++

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

	body, contentType := multipartBody(
		t,
		"file",
		"large.torrent",
		io.LimitReader(zeroReader{}, maxTorrentUploadBodyBytes+1024),
	)

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

	if torrentsSvc.addCalls > 0 {
		t.Fatal("oversized upload should be rejected before torrent service is called")
	}
}

func TestTorrentUploadSingleFileReturnsLegacyObject(t *testing.T) {
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

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected single-file upload to return JSON object: %v body=%s", err, w.Body.String())
	}

	if got["title"] != "uploaded-1" {
		t.Fatalf("expected uploaded-1 title, got %#v", got["title"])
	}

	if torrentsSvc.addCalls != 1 {
		t.Fatal("expected torrent service Add to be called")
	}

	if torrentsSvc.finalizeCalls != 1 {
		t.Fatal("expected metadata finalization to be queued")
	}
}

func TestTorrentUploadMultipleFilesReturnsArray(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &uploadTorrentService{}

	body, contentType := multipartBodyWithFiles(t,
		multipartUploadFile{"file", "first.torrent", bytes.NewReader(validTorrentBytes(t))},
		multipartUploadFile{"file", "second.torrent", bytes.NewReader(validTorrentBytes(t))},
	)

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

	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected multi-file upload to return JSON array: %v body=%s", err, w.Body.String())
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 statuses, got %d body=%s", len(got), w.Body.String())
	}

	if torrentsSvc.addCalls != 2 {
		t.Fatalf("expected 2 Add calls, got %d", torrentsSvc.addCalls)
	}

	if torrentsSvc.finalizeCalls != 2 {
		t.Fatalf("expected 2 finalize calls, got %d", torrentsSvc.finalizeCalls)
	}
}

func TestTorrentUploadMultipleFilesKeepsArrayWhenOneFileFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &uploadTorrentService{
		addErrors: map[int]error{
			2: errors.New("duplicate torrent"),
		},
	}

	body, contentType := multipartBodyWithFiles(t,
		multipartUploadFile{"file", "first.torrent", bytes.NewReader(validTorrentBytes(t))},
		multipartUploadFile{"file", "duplicate.torrent", bytes.NewReader(validTorrentBytes(t))},
	)

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

	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected partial multi-file upload to keep JSON array response: %v body=%s", err, w.Body.String())
	}

	if len(got) != 1 {
		t.Fatalf("expected one successful status, got %d body=%s", len(got), w.Body.String())
	}

	if torrentsSvc.addCalls != 2 {
		t.Fatalf("expected both files to be attempted, got %d Add calls", torrentsSvc.addCalls)
	}

	if torrentsSvc.finalizeCalls != 1 {
		t.Fatalf("expected only successful torrent to finalize, got %d", torrentsSvc.finalizeCalls)
	}
}

func TestTorrentUploadReturnsBadRequestWhenAllFilesFail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	torrentsSvc := &uploadTorrentService{
		addErrors: map[int]error{
			1: errors.New("parse duplicate"),
		},
	}

	body, contentType := multipartBody(t, "file", "duplicate.torrent", bytes.NewReader(validTorrentBytes(t)))

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

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}

	if torrentsSvc.finalizeCalls != 0 {
		t.Fatalf("expected no finalize calls, got %d", torrentsSvc.finalizeCalls)
	}
}

func TestTorrentUploadRemovesTemporaryMultipartFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	body, contentType := multipartBody(
		t,
		"file",
		"spilled.torrent",
		bytes.NewReader(bytes.Repeat(validTorrentBytes(t), 64)),
	)

	r := gin.New()
	r.MaxMultipartMemory = 32
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{
		Torrents: &uploadTorrentService{},
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

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected multipart temp files to be removed, found %d", len(entries))
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

	return multipartBodyWithFiles(t, multipartUploadFile{fieldName, fileName, src})
}

type multipartUploadFile struct {
	fieldName string
	fileName  string
	src       io.Reader
}

func multipartBodyWithFiles(t *testing.T, files ...multipartUploadFile) (*bytes.Buffer, string) {
	t.Helper()

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	for _, file := range files {
		part, err := writer.CreateFormFile(file.fieldName, file.fileName)
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}

		if _, err := io.Copy(part, file.src); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
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
