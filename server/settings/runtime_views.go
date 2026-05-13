package settings

type RuntimePathConfig struct {
	Path string
}

type RuntimeServerConfig struct {
	IP      string
	Port    string
	SSL     bool
	SSLPort string
	MaxSize int64
}

type RuntimeAuthConfig struct {
	HTTPAuth bool
	SearchWA bool
}

type RuntimeIdentityConfig struct {
	PubIPv4 string
	PubIPv6 string
	TorAddr string
}

func (s RuntimeState) PathConfig() RuntimePathConfig {
	return RuntimePathConfig{
		Path: s.Path,
	}
}

func (s RuntimeState) ServerConfig() RuntimeServerConfig {
	return RuntimeServerConfig{
		IP:      s.IP,
		Port:    s.Port,
		SSL:     s.Ssl,
		SSLPort: s.SslPort,
		MaxSize: s.MaxSize,
	}
}

func (s RuntimeState) AuthConfig() RuntimeAuthConfig {
	return RuntimeAuthConfig{
		HTTPAuth: s.HTTPAuth,
		SearchWA: s.SearchWA,
	}
}

func (s RuntimeState) IdentityConfig() RuntimeIdentityConfig {
	return RuntimeIdentityConfig{
		PubIPv4: s.PubIPv4,
		PubIPv6: s.PubIPv6,
		TorAddr: s.TorAddr,
	}
}
