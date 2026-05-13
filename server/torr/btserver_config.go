package torr

import (
	"context"
	"net"

	"github.com/anacrolix/publicip"
	"github.com/anacrolix/torrent"
	"github.com/wlynxg/anet"

	"server/log"
	"server/proxy"
	"server/settings"
	"server/torr/storage/torrstor"
	"server/torr/utils"
	"server/version"
)

// buildClientConfig creates and configures the torrent client configuration
// based on current application settings.
func (bt *BTServer) buildClientConfig() *torrent.ClientConfig {
	config := torrent.NewDefaultClientConfig()
	sets := bt.currentSettings()
	identityCfg := bt.currentRuntimeState().IdentityConfig()
	cacheCfg := sets.CacheConfig()
	networkCfg := sets.NetworkConfig()
	debugCfg := sets.DebugConfig()

	storage := torrstor.NewStorageWithProvider(cacheCfg.SizeBytes, bt.deps.settingsProvider)
	bt.storage = storage
	config.DefaultStorage = storage

	userAgent := "qBittorrent/4.3.9"
	peerID := "-qB4390-"
	upnpID := "TorrServer/" + version.Version()
	cliVers := userAgent

	config.Debug = debugCfg.EnableDebug && !debugCfg.ServiceOnlyDebug
	config.DisableIPv6 = !networkCfg.EnableIPv6
	config.DisableTCP = networkCfg.DisableTCP
	config.DisableUTP = networkCfg.DisableUTP
	config.NoDefaultPortForwarding = networkCfg.DisableUPNP
	config.NoDHT = networkCfg.DisableDHT
	config.DisablePEX = networkCfg.DisablePEX
	config.NoUpload = networkCfg.DisableUpload
	config.Bep20 = peerID
	config.PeerID = utils.PeerIDRandom(peerID)
	config.UpnpID = upnpID
	config.HTTPUserAgent = userAgent
	config.ExtendedHandshakeClientVersion = cliVers
	effectiveConns := effectiveEstablishedConns(networkCfg.ConnectionsLimit, config.EstablishedConnsPerTorrent)
	config.EstablishedConnsPerTorrent = effectiveConns
	config.HalfOpenConnsPerTorrent = maxInt(effectiveConns, config.HalfOpenConnsPerTorrent)
	config.TotalHalfOpenConns = maxInt(effectiveConns*8, 200)
	config.TorrentPeersLowWater, config.TorrentPeersHighWater = peerWatermarks(effectiveConns)

	if networkCfg.ForceEncrypt {
		config.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{
			RequirePreferred: true,
			Preferred:        true,
		}
	}

	if networkCfg.DownloadRateLimitKB > 0 {
		config.DownloadRateLimiter = utils.Limit(networkCfg.DownloadRateLimitKB * 1024)
	}

	if networkCfg.UploadRateLimitKB > 0 {
		config.Seed = true
		config.UploadRateLimiter = utils.Limit(networkCfg.UploadRateLimitKB * 1024)
	}

	if identityCfg.TorAddr != "" {
		log.TLogln("Set listen addr", identityCfg.TorAddr)
		config.SetListenAddr(identityCfg.TorAddr)
	} else if networkCfg.PeersListenPort > 0 {
		log.TLogln("Set listen port", networkCfg.PeersListenPort)
		config.ListenPort = networkCfg.PeersListenPort
	} else {
		log.TLogln("Set listen port to random autoselect (0)")
		config.ListenPort = 0
	}

	return config
}

func effectiveEstablishedConns(userLimit, defaultConns int) int {
	if defaultConns <= 0 {
		defaultConns = 50
	}

	if userLimit <= 0 {
		return defaultConns
	}

	if userLimit < defaultConns {
		return defaultConns
	}

	return userLimit
}

func peerWatermarks(effectiveConns int) (int, int) {
	if effectiveConns <= 0 {
		effectiveConns = 25
	}

	low := maxInt(effectiveConns*2, 50)
	high := maxInt(effectiveConns*10, 500)

	if high < low+50 {
		high = low + 50
	}

	return low, high
}

// detectPublicIPv4 detects the public IPv4 address for the torrent client.
func detectPublicIPv4(ctx context.Context, config *torrent.ClientConfig, identityCfg settings.RuntimeIdentityConfig) {
	if identityCfg.PubIPv4 != "" {
		if ip4 := net.ParseIP(identityCfg.PubIPv4); ip4.To4() != nil && !isPrivateIP(ip4) {
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
func detectPublicIPv6(ctx context.Context, config *torrent.ClientConfig, identityCfg settings.RuntimeIdentityConfig, enableIPv6 bool) bool {
	if !enableIPv6 {
		return false
	}
	if identityCfg.PubIPv6 != "" {
		if ip6 := net.ParseIP(identityCfg.PubIPv6); ip6.To16() != nil && ip6.To4() == nil && !isPrivateIP(ip6) {
			config.PublicIp6 = ip6
			return true
		}
	}

	if config.PublicIp6 == nil {
		ip, err := publicip.Get6(ctx)
		if err != nil {
			log.TLogln("Error getting public ipv6 address:", err)
			return false
		}
		if ip.To16() == nil {
			return false
		}

		config.PublicIp6 = ip
	}

	if config.PublicIp6 != nil {
		log.TLogln("PublicIp6:", config.PublicIp6)
		return true
	}

	return false
}

func (bt *BTServer) configure(ctx context.Context) {
	bt.config = bt.buildClientConfig()
	sets := bt.currentSettings()

	if err := bt.configureProxy(); err != nil {
		log.TLogln("Proxy configuration error:", err)
	}

	log.TLogln("Client config:", sets)
	identityCfg := bt.currentRuntimeState().IdentityConfig()
	detectPublicIPv4(ctx, bt.config, identityCfg)

	if !detectPublicIPv6(ctx, bt.config, identityCfg, sets.EnableIPv6) && sets.EnableIPv6 {
		bt.config.DisableIPv6 = true
		log.TLogln("IPv6 disabled: public IPv6 is unavailable")
	}
}

func (bt *BTServer) configureProxy() error {
	args := bt.currentArgs()
	if args == nil {
		return nil
	}

	proxyCfg, err := proxy.NewConfig(args.ProxyURL, args.ProxyMode)
	if err != nil {
		return err
	}

	if proxyCfg == nil {
		return nil
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
		log.TLogln("Configuring HTTP proxy for tracker requests (default):", proxyCfg.URL.String())
		bt.config.HTTPProxy = d.HTTPProxy()
	}

	return nil
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
