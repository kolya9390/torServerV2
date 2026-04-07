package torr

import (
	"context"
	"fmt"
	"maps"
	"net"
	"sync"

	"github.com/anacrolix/publicip"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/wlynxg/anet"

	"server/log"
	"server/proxy"
	"server/settings"
	"server/torr/storage/torrstor"
	"server/torr/utils"
	"server/version"
)

type BTServer struct {
	config *torrent.ClientConfig
	client *torrent.Client

	storage *torrstor.Storage

	torrents map[metainfo.Hash]*Torrent

	mu sync.Mutex
}

var privateIPBlocks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Errorf("parse error on %q: %v", cidr, err))
		}

		privateIPBlocks = append(privateIPBlocks, block)
	}
}

func NewBTS() *BTServer {
	bts := new(BTServer)
	bts.torrents = make(map[metainfo.Hash]*Torrent)

	return bts
}

func (bt *BTServer) Connect() error {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	var err error

	bt.configure(context.TODO())
	bt.client, err = torrent.NewClient(bt.config)
	bt.torrents = make(map[metainfo.Hash]*Torrent)
	InitAPIHelper(bt)

	return err
}

func (bt *BTServer) Disconnect() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	if bt.client != nil {
		bt.client.Close()
		bt.client = nil

		utils.FreeOSMemGC()
	}
}

// buildClientConfig creates and configures the torrent client configuration
// based on current application settings.
func (bt *BTServer) buildClientConfig() *torrent.ClientConfig {
	blocklist, err := utils.ReadBlockedIP()
	if err != nil {
		log.TLogln("Error reading blocked IPs:", err)
	}

	config := torrent.NewDefaultClientConfig()

	storage := torrstor.NewStorage(settings.BTsets.CacheSize)
	bt.storage = storage
	config.DefaultStorage = storage

	userAgent := "qBittorrent/4.3.9"
	peerID := "-qB4390-"
	upnpID := "TorrServer/" + version.Version
	cliVers := userAgent

	config.Debug = settings.BTsets.EnableDebug
	config.DisableIPv6 = !settings.BTsets.EnableIPv6
	config.DisableTCP = settings.BTsets.DisableTCP
	config.DisableUTP = settings.BTsets.DisableUTP
	config.NoDefaultPortForwarding = settings.BTsets.DisableUPNP
	config.NoDHT = settings.BTsets.DisableDHT
	config.DisablePEX = settings.BTsets.DisablePEX
	config.NoUpload = settings.BTsets.DisableUpload
	config.IPBlocklist = blocklist
	config.Bep20 = peerID
	config.PeerID = utils.PeerIDRandom(peerID)
	config.UpnpID = upnpID
	config.HTTPUserAgent = userAgent
	config.ExtendedHandshakeClientVersion = cliVers
	config.EstablishedConnsPerTorrent = settings.BTsets.ConnectionsLimit

	if settings.BTsets.ForceEncrypt {
		config.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{
			RequirePreferred: true,
			Preferred:        true,
		}
	}

	if settings.BTsets.DownloadRateLimit > 0 {
		config.DownloadRateLimiter = utils.Limit(settings.BTsets.DownloadRateLimit * 1024)
	}

	if settings.BTsets.UploadRateLimit > 0 {
		config.Seed = true
		config.UploadRateLimiter = utils.Limit(settings.BTsets.UploadRateLimit * 1024)
	}

	if settings.TorAddr != "" {
		log.TLogln("Set listen addr", settings.TorAddr)
		config.SetListenAddr(settings.TorAddr)
	} else if settings.BTsets.PeersListenPort > 0 {
		log.TLogln("Set listen port", settings.BTsets.PeersListenPort)
		config.ListenPort = settings.BTsets.PeersListenPort
	} else {
		log.TLogln("Set listen port to random autoselect (0)")

		config.ListenPort = 0
	}

	return config
}

// detectPublicIPv4 detects the public IPv4 address for the torrent client.
// It first checks the configured setting, then falls back to external discovery.
func detectPublicIPv4(ctx context.Context, config *torrent.ClientConfig) {
	if settings.PubIPv4 != "" {
		if ip4 := net.ParseIP(settings.PubIPv4); ip4.To4() != nil && !isPrivateIP(ip4) {
			config.PublicIp4 = ip4

			return
		}
	}

	if config.PublicIp4 == nil {
		ip, err := publicip.Get4(ctx)
		if err != nil {
			log.TLogln("Error getting public ipv4 address:", err)

			return
		}
		// publicip.Get4 can return IPv6 in some cases, validate
		if ip.To4() == nil {
			return
		}

		config.PublicIp4 = ip
	}

	if config.PublicIp4 != nil {
		log.TLogln("PublicIp4:", config.PublicIp4)
	}
}

// detectPublicIPv6 detects the public IPv6 address for the torrent client.
// It first checks the configured setting, then falls back to external discovery.
func detectPublicIPv6(ctx context.Context, config *torrent.ClientConfig, enableIPv6 bool) {
	if settings.PubIPv6 != "" {
		if ip6 := net.ParseIP(settings.PubIPv6); ip6.To16() != nil && ip6.To4() == nil && !isPrivateIP(ip6) {
			config.PublicIp6 = ip6

			return
		}
	}

	if config.PublicIp6 == nil && enableIPv6 {
		ip, err := publicip.Get6(ctx)
		if err != nil {
			log.TLogln("Error getting public ipv6 address:", err)

			return
		}
		// Ensure it's valid IPv6
		if ip.To16() == nil {
			return
		}

		config.PublicIp6 = ip
	}

	if config.PublicIp6 != nil {
		log.TLogln("PublicIp6:", config.PublicIp6)
	}
}

func (bt *BTServer) configure(ctx context.Context) {
	bt.config = bt.buildClientConfig()

	// Configure proxy if enabled
	if err := bt.configureProxy(); err != nil {
		log.TLogln("Proxy configuration error:", err)
	}

	log.TLogln("Client config:", settings.BTsets)

	// Detect public IP addresses
	detectPublicIPv4(ctx, bt.config)
	detectPublicIPv6(ctx, bt.config, settings.BTsets.EnableIPv6)
}

func (bt *BTServer) configureProxy() error {
	args := settings.GetArgs()
	if args == nil {
		return nil
	}

	proxyCfg, err := proxy.NewConfig(args.ProxyURL, args.ProxyMode)
	if err != nil {
		return err
	}

	if proxyCfg == nil {
		return nil // No proxy configured
	}

	d := proxy.NewDialer(proxyCfg)

	switch proxyCfg.Mode {
	case proxy.ModeTracker:
		log.TLogln("Configuring HTTP proxy for tracker requests:", proxyCfg.URL.String())

		bt.config.HTTPProxy = d.HTTPProxy()

		log.TLogln("Proxy configured successfully for HTTP tracker connections only")

	case proxy.ModePeers, proxy.ModeFull:
		log.TLogln("Configuring proxy for all connections:", proxyCfg.URL.String())

		bt.config.HTTPDialContext = d.DialContext
		bt.config.HTTPProxy = d.HTTPProxy()

		log.TLogln("Proxy configured successfully for all BitTorrent connections")

	default:
		// Fallback to tracker mode for unknown modes
		log.TLogln("Configuring HTTP proxy for tracker requests (default):", proxyCfg.URL.String())

		bt.config.HTTPProxy = d.HTTPProxy()
	}

	return nil
}

func (bt *BTServer) GetTorrent(hash torrent.InfoHash) *Torrent {
	if torr, ok := bt.torrents[hash]; ok {
		return torr
	}

	return nil
}

func (bt *BTServer) ListTorrents() map[metainfo.Hash]*Torrent {
	list := make(map[metainfo.Hash]*Torrent)
	maps.Copy(list, bt.torrents)

	return list
}

func (bt *BTServer) RemoveTorrent(hash torrent.InfoHash) bool {
	if torr, ok := bt.torrents[hash]; ok {
		return torr.Close()
	}

	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

// getPublicIP4 returns the first non-private IPv4 address of all active interfaces.
// It returns nil if no suitable address is found.
//
//nolint:unused // Reserved for debugging public IP addresses
func getPublicIP4() net.IP {
	ifaces, err := anet.Interfaces()
	if err != nil {
		log.TLogln("Error get public IPv4:", err)

		return nil
	}

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

				if !isPrivateIP(ip) && ip.To4() != nil {
					return ip
				}
			}
		}
	}

	return nil
}

// getPublicIP6 returns the first non-private IPv6 address of all active interfaces.
// It returns nil if no suitable address is found.
//
//nolint:unused // Reserved for debugging public IP addresses
func getPublicIP6() net.IP {
	ifaces, err := anet.Interfaces()
	if err != nil {
		log.TLogln("Error get public IPv6:", err)

		return nil
	}

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

				if !isPrivateIP(ip) && ip.To16() != nil && ip.To4() == nil {
					return ip
				}
			}
		}
	}

	return nil
}
