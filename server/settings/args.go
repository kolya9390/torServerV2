package settings

import "sync"

type ExecArgs struct {
	Port        string
	IP          string
	Ssl         bool
	SslPort     string
	SslCert     string
	SslKey      string
	Path        string
	LogPath     string
	WebLogPath  string
	RDB         bool
	HttpAuth    bool
	DontKill    bool
	UI          bool
	TorrentsDir string
	TorrentAddr string
	PubIPv4     string
	PubIPv6     string
	SearchWA    bool
	MaxSize     string
	TGToken     string
	FusePath    string
	WebDAV      bool
	ProxyURL    string
	ProxyMode   string
}

var Args *ExecArgs
var argsMu sync.RWMutex

// SetArgs stores execution args as a runtime snapshot.
func SetArgs(args *ExecArgs) {
	if args == nil {
		return
	}

	cp := *args

	argsMu.Lock()
	Args = &cp
	argsMu.Unlock()
}

// GetArgs returns a copy of current execution args.
func GetArgs() *ExecArgs {
	argsMu.RLock()
	defer argsMu.RUnlock()

	if Args == nil {
		return nil
	}

	cp := *Args

	return &cp
}
