package settings

// Transitional compatibility layer for legacy runtime package variables.
// New code should prefer RuntimeState accessors instead of reading these directly.
var (
	Path     string
	IP       string
	Port     string
	Ssl      bool
	SslPort  string
	HTTPAuth bool
	SearchWA bool
	PubIPv4  string
	PubIPv6  string
	TorAddr  string
	MaxSize  int64
)

func syncLegacyRuntimeVars(state RuntimeState) {
	Path = state.Path
	IP = state.IP
	Port = state.Port
	Ssl = state.Ssl
	SslPort = state.SslPort
	HTTPAuth = state.HTTPAuth
	SearchWA = state.SearchWA
	PubIPv4 = state.PubIPv4
	PubIPv6 = state.PubIPv6
	TorAddr = state.TorAddr
	MaxSize = state.MaxSize
}

func legacyRuntimeStateSnapshot() RuntimeState {
	return RuntimeState{
		Path:     Path,
		IP:       IP,
		Port:     Port,
		Ssl:      Ssl,
		SslPort:  SslPort,
		HTTPAuth: HTTPAuth,
		SearchWA: SearchWA,
		PubIPv4:  PubIPv4,
		PubIPv6:  PubIPv6,
		TorAddr:  TorAddr,
		MaxSize:  MaxSize,
	}
}
