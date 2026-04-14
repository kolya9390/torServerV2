package webinfra

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/wlynxg/anet"

	"server/log"
	"server/settings"
	"server/web/sslcerts"
)

// CORSConfig is an alias for gin-contrib cors configuration.
type CORSConfig = cors.Config

// CORSService defines the interface for building CORS configuration.
type CORSService interface {
	BuildConfig() CORSConfig
	GetAllowedOrigins() []string
}

// SSLService defines the interface for managing SSL certificates and HTTPS server.
type SSLService interface {
	PrepareCertificates(ips []string) error
	VerifyOrRegenerateCerts(ips []string) error
	Server(addr string, handler http.Handler) *http.Server
}

// corsService implements CORSService.
type corsService struct{}

// sslService implements SSLService.
type sslService struct {
	mu   sync.Mutex
	cert string
	key  string
	srv  *http.Server
}

// NewCORSService creates a new instance of CORSService.
func NewCORSService() CORSService {
	return &corsService{}
}

// NewSSLService creates a new instance of SSLService.
func NewSSLService() SSLService {
	return &sslService{}
}

// BuildConfig constructs the CORS configuration based on environment variables and smart defaults.
func (c *corsService) BuildConfig() CORSConfig {
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "X-Requested-With", "Accept", "Authorization"}
	corsCfg.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	corsCfg.AllowCredentials = true

	// 1. Check if strict origin list is configured (Best for VPS/Internet exposed servers)
	if os.Getenv("TS_CORS_ALLOW_ORIGINS") != "" {
		corsCfg.AllowOriginFunc = func(origin string) bool {
			if origin == "" {
				return true
			}

			for _, allowed := range c.GetAllowedOrigins() {
				if allowed == origin {
					return true
				}
			}
			// Also allow local network origins automatically
			return isLocalOrigin(origin)
		}

		log.TLogln("CORS mode: strict (configured origins + local network)")
	} else {
		// 2. Allow all origins by default (Standard for Home Media Servers)
		// This ensures 100% compatibility with Smart TVs (Lampa, Kodi, etc.) which often
		// send non-standard Origin headers (file://, app://, null, etc.).
		// Real security is handled by IP Blocker (wip.txt) and HTTP Auth.
		corsCfg.AllowAllOrigins = true

		log.TLogln("CORS mode: allow-all (compatible with Smart TV apps)")
	}

	if os.Getenv("TS_CORS_ALLOW_PRIVATE_NETWORK") == "1" {
		corsCfg.AllowPrivateNetwork = true

		log.TLogln("CORS private network allowed (TS_CORS_ALLOW_PRIVATE_NETWORK=1)")
	}

	return corsCfg
}

// isLocalOrigin checks if the origin belongs to a private or loopback IP range.
func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := u.Hostname()

	// 1. Explicitly allow common local hostnames (net.ParseIP does not resolve 'localhost')
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}

	// 2. Allow local file origins (common for Smart TV apps like Lampa)
	// This covers "file://", "null" origin, or "capacitor://localhost"
	if host == "" || strings.ToLower(u.Scheme) == "capacitor" {
		return true
	}

	// 3. Check for private/loopback IPs (192.168.x.x, 10.x.x.x)
	ip := net.ParseIP(host)

	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}

// GetAllowedOrigins returns the list of explicitly allowed origins from env vars or settings.
func (c *corsService) GetAllowedOrigins() []string {
	if raw := strings.TrimSpace(os.Getenv("TS_CORS_ALLOW_ORIGINS")); raw != "" {
		parts := strings.Split(raw, ",")
		origins := make([]string, 0, len(parts))

		for _, part := range parts {
			origin := strings.TrimSpace(part)
			if origin != "" {
				origins = append(origins, origin)
			}
		}

		if len(origins) > 0 {
			return origins
		}
	}

	uniq := make(map[string]struct{})
	add := func(origin string) {
		if origin != "" {
			uniq[origin] = struct{}{}
		}
	}

	// Add localhost and configured IP to the allowlist
	add("http://127.0.0.1:" + settings.Port)
	add("http://localhost:" + settings.Port)

	if settings.Ssl && settings.SslPort != "" {
		add("https://127.0.0.1:" + settings.SslPort)
		add("https://localhost:" + settings.SslPort)
	}

	if settings.IP != "" && settings.IP != "0.0.0.0" && settings.IP != "::" {
		add(fmt.Sprintf("http://%s:%s", settings.IP, settings.Port))

		if settings.Ssl && settings.SslPort != "" {
			add(fmt.Sprintf("https://%s:%s", settings.IP, settings.SslPort))
		}
	}

	origins := make([]string, 0, len(uniq))
	for origin := range uniq {
		origins = append(origins, origin)
	}

	sort.Strings(origins)

	return origins
}

// PrepareCertificates generates or retrieves SSL certificates for the given IPs.
func (s *sslService) PrepareCertificates(ips []string) error {
	if !settings.Ssl {
		return nil
	}

	if settings.GetSettings().SslCert != "" && settings.GetSettings().SslKey != "" {
		return nil
	}

	cert, key, err := sslcerts.MakeCertKeyFiles(ips)
	if err != nil {
		return fmt.Errorf("unable to generate certificate and key: %w", err)
	}

	s.mu.Lock()
	s.cert = cert
	s.key = key
	s.mu.Unlock()

	settings.GetSettings().SslCert, settings.GetSettings().SslKey = cert, key
	log.TLogln("Saving path to ssl cert and key in db", settings.GetSettings().SslCert, settings.GetSettings().SslKey)
	settings.SetBTSets(settings.BTsets)

	return nil
}

// VerifyOrRegenerateCerts checks if current certificates are valid and regenerates them if needed.
func (s *sslService) VerifyOrRegenerateCerts(ips []string) error {
	if !settings.Ssl {
		return nil
	}

	err := sslcerts.VerifyCertKeyFiles(settings.GetSettings().SslCert, settings.GetSettings().SslKey, settings.SslPort)
	if err == nil {
		return nil
	}

	log.TLogln("Error checking certificate and private key files:", err)

	cert, key, certErr := sslcerts.MakeCertKeyFiles(ips)
	if certErr != nil {
		return fmt.Errorf("unable to re-generate certificate and key: %w", certErr)
	}

	s.mu.Lock()
	s.cert = cert
	s.key = key
	s.mu.Unlock()

	settings.GetSettings().SslCert, settings.GetSettings().SslKey = cert, key
	log.TLogln("Saving path to ssl cert and key in db", settings.GetSettings().SslCert, settings.GetSettings().SslKey)
	settings.SetBTSets(settings.BTsets)

	return nil
}

// Server creates or returns a cached HTTPS server instance.
func (s *sslService) Server(addr string, handler http.Handler) *http.Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.srv != nil {
		return s.srv
	}

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return s.srv
}

// GetLocalIps returns a list of non-loopback, non-link-local IP addresses for this host.
func GetLocalIps() []string {
	ifaces, err := anet.Interfaces()
	if err != nil {
		log.TLogln("Error get local IPs")

		return nil
	}

	var list []string

	for _, i := range ifaces {
		addrs, err := anet.InterfaceAddrsByInterface(&i)
		if err != nil {
			continue
		}

		if i.Flags&net.FlagUp == net.FlagUp {
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}

				if !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() {
					list = append(list, ip.String())
				}
			}
		}
	}

	sort.Strings(list)

	return list
}

// CheckTrustedProxies returns the list of trusted proxy IPs, defaulting to localhost.
func CheckTrustedProxies() []string {
	trustedProxies := []string{"127.0.0.1", "::1"}

	if val := strings.TrimSpace(os.Getenv("TS_TRUSTED_PROXIES")); val != "" {
		var configured []string

		for part := range strings.SplitSeq(val, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				configured = append(configured, part)
			}
		}

		if len(configured) > 0 {
			trustedProxies = configured
		}
	}

	return trustedProxies
}

// SetTrustedProxies configures the trusted proxies for the given router.
func SetTrustedProxies(route interface{ SetTrustedProxies(proxies []string) error }, proxies []string) error {
	return route.SetTrustedProxies(proxies)
}

var (
	ErrServerNotRunning = errors.New("server not running")
	ErrInvalidCert      = errors.New("invalid certificate")
)
