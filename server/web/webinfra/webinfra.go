package webinfra

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
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

type CORSConfig = cors.Config

type CORSService interface {
	BuildConfig() CORSConfig
	GetAllowedOrigins() []string
}

type SSLService interface {
	PrepareCertificates(ips []string) error
	VerifyOrRegenerateCerts(ips []string) error
	Server(addr string, handler http.Handler) *http.Server
}

type corsService struct{}

type sslService struct {
	mu   sync.Mutex
	cert string
	key  string
	srv  *http.Server
}

func NewCORSService() CORSService {
	return &corsService{}
}

func NewSSLService() SSLService {
	return &sslService{}
}

func (c *corsService) BuildConfig() CORSConfig {
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "X-Requested-With", "Accept", "Authorization"}
	corsCfg.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}

	if os.Getenv("TS_CORS_ALLOW_ALL") == "1" {
		corsCfg.AllowAllOrigins = true

		log.TLogln("CORS mode: allow-all (TS_CORS_ALLOW_ALL=1)")
	} else {
		corsCfg.AllowOrigins = c.GetAllowedOrigins()
		log.TLogln("CORS mode: allowlist", corsCfg.AllowOrigins)
	}

	if os.Getenv("TS_CORS_ALLOW_PRIVATE_NETWORK") == "1" {
		corsCfg.AllowPrivateNetwork = true

		log.TLogln("CORS private network allowed (TS_CORS_ALLOW_PRIVATE_NETWORK=1)")
	}

	return corsCfg
}

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

func (s *sslService) PrepareCertificates(ips []string) error {
	if !settings.Ssl {
		return nil
	}

	if settings.BTsets.SslCert != "" && settings.BTsets.SslKey != "" {
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

	settings.BTsets.SslCert, settings.BTsets.SslKey = cert, key
	log.TLogln("Saving path to ssl cert and key in db", settings.BTsets.SslCert, settings.BTsets.SslKey)
	settings.SetBTSets(settings.BTsets)

	return nil
}

func (s *sslService) VerifyOrRegenerateCerts(ips []string) error {
	if !settings.Ssl {
		return nil
	}

	err := sslcerts.VerifyCertKeyFiles(settings.BTsets.SslCert, settings.BTsets.SslKey, settings.SslPort)
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

	settings.BTsets.SslCert, settings.BTsets.SslKey = cert, key
	log.TLogln("Saving path to ssl cert and key in db", settings.BTsets.SslCert, settings.BTsets.SslKey)
	settings.SetBTSets(settings.BTsets)

	return nil
}

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

func SetTrustedProxies(route interface{ SetTrustedProxies(proxies []string) error }, proxies []string) error {
	return route.SetTrustedProxies(proxies)
}

var (
	ErrServerNotRunning = errors.New("server not running")
	ErrInvalidCert      = errors.New("invalid certificate")
)
