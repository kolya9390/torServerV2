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
