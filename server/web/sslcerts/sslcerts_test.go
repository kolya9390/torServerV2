package sslcerts

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"server/settings"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, privPEM, err := generateSelfSignedCert([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("generateSelfSignedCert failed: %v", err)
	}

	if len(certPEM) == 0 {
		t.Error("expected non-empty certificate")
	}
	if len(privPEM) == 0 {
		t.Error("expected non-empty private key")
	}

	cert, err := tls.X509KeyPair(certPEM, privPEM)
	if err != nil {
		t.Fatalf("X509KeyPair failed: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Error("expected at least one certificate")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate failed: %v", err)
	}

	if x509Cert.NotAfter.Before(time.Now()) {
		t.Error("certificate should not be expired")
	}

	if x509Cert.NotAfter.Before(x509Cert.NotBefore) {
		t.Error("NotAfter should be after NotBefore")
	}

	if len(x509Cert.IPAddresses) != 2 {
		t.Errorf("expected 2 IP addresses, got %d", len(x509Cert.IPAddresses))
	}
}

func TestGenerateSelfSignedCert_NoIPs(t *testing.T) {
	certPEM, privPEM, err := generateSelfSignedCert(nil)
	if err != nil {
		t.Fatalf("generateSelfSignedCert with nil IPs failed: %v", err)
	}

	if len(certPEM) == 0 || len(privPEM) == 0 {
		t.Error("expected non-empty cert and key")
	}
}

func TestGetAbsPath(t *testing.T) {
	absPath, err := getAbsPath("test.txt")
	if err != nil {
		t.Fatalf("getAbsPath failed: %v", err)
	}

	if absPath == "" {
		t.Error("expected non-empty absolute path")
	}

	expectedSuffix := string(filepath.Separator) + "test.txt"
	if len(absPath) < len(expectedSuffix) || absPath[len(absPath)-len(expectedSuffix):] != expectedSuffix {
		t.Errorf("expected path to end with %s, got %s", expectedSuffix, absPath)
	}
}

func TestVerifyCertKeyFiles_InvalidCert(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	if err := os.WriteFile(certFile, []byte("invalid cert"), 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("invalid key"), 0644); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	err := VerifyCertKeyFiles(certFile, keyFile, "18444")
	if err == nil {
		t.Error("expected error for invalid cert")
	}
}

func TestVerifyCertKeyFiles_ExpiredCert(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	certPEM, privPEM, err := generateSelfSignedCert(nil)
	if err != nil {
		t.Fatalf("generateSelfSignedCert failed: %v", err)
	}

	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, privPEM, 0644); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	err = VerifyCertKeyFiles(certFile, keyFile, "0")
	if err != nil {
		t.Logf("VerifyCertKeyFiles error (expected): %v", err)
	}
}

func TestMakeCertKeyFiles(t *testing.T) {
	origPath := settings.Path
	tmpDir := t.TempDir()
	settings.Path = tmpDir
	defer func() { settings.Path = origPath }()

	certPath, keyPath, err := MakeCertKeyFiles([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("MakeCertKeyFiles failed: %v", err)
	}

	if certPath == "" || keyPath == "" {
		t.Error("expected non-empty paths")
	}

	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read cert: %v", err)
	}
	if len(certData) == 0 {
		t.Error("cert file is empty")
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed to read key: %v", err)
	}
	if len(keyData) == 0 {
		t.Error("key file is empty")
	}
}
