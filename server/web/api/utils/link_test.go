package utils

import (
	"testing"
)

func TestParseLink_Magnet(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=torrent"

	spec, err := ParseLink(magnet)
	if err != nil {
		t.Fatalf("ParseLink failed: %v", err)
	}

	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
}

func TestParseLink_InfoHashOnly(t *testing.T) {
	infoHash := "c12fe1c06bba254a9dc9f519b335aa7c1367a88a"

	spec, err := ParseLink(infoHash)
	if err != nil {
		t.Fatalf("ParseLink failed: %v", err)
	}

	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
}

func TestParseLink_UnknownScheme(t *testing.T) {
	_, err := ParseLink("ftp://example.com/torrent")
	if err == nil {
		t.Error("expected error for unknown scheme")
	}
}

func TestParseLink_InvalidURL(t *testing.T) {
	_, err := ParseLink("://invalid")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestParseTorrsHash_Invalid(t *testing.T) {
	_, _, err := ParseTorrsHash("invalid_token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestParseTorrsHash_WithPrefix(t *testing.T) {
	_, _, err := ParseTorrsHash("torrs://invalid_token")
	if err == nil {
		t.Error("expected error for invalid torrs hash")
	}
}

func TestFromMagnet(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:c12fe1c06bba254a9dc9f519b335aa7c1367a88a&dn=test"

	spec, err := fromMagnet(magnet)
	if err != nil {
		t.Fatalf("fromMagnet failed: %v", err)
	}

	if spec == nil {
		t.Fatal("expected non-nil spec")
	}

	if len(spec.DisplayName) == 0 {
		t.Error("expected non-empty DisplayName")
	}
}

func TestFromMagnet_Invalid(t *testing.T) {
	_, err := fromMagnet("not_a_magnet")
	if err == nil {
		t.Error("expected error for invalid magnet")
	}
}

func TestParseLink_File(t *testing.T) {
	_, err := ParseLink("file:///nonexistent/path.torrent")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
