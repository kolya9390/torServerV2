package dlna

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/log"
	"github.com/wlynxg/anet"

	"server/settings"
)

var dmsServer *dms.Server

func Start() error {
	logger := log.Default.WithNames("dlna")

	var connErr error

	dmsServer = &dms.Server{
		Logger: logger.WithNames("dms", "server"),
		Interfaces: func() (ifs []net.Interface) {
			var err error
			ifaces, err := anet.Interfaces()
			if err != nil {
				logger.Levelf(log.Error, "%v", err)

				return
			}
			for _, i := range ifaces {
				// interface flags seem to always be 0 on Windows
				if runtime.GOOS != "windows" && (i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagMulticast == 0) {
					continue
				}
				ifs = append(ifs, i)
			}

			return
		}(),
		HTTPConn: func() net.Listener {
			port := 9080
			for {
				logger.Levelf(log.Info, "Check dlna port %d", port)
				m, err := net.Listen("tcp", settings.IP+":"+strconv.Itoa(port))
				if m != nil {
					_ = m.Close()
				}
				if err == nil {
					break
				}
				port++
			}
			logger.Levelf(log.Info, "Set dlna port %d", port)
			conn, err := net.Listen("tcp", settings.IP+":"+strconv.Itoa(port))
			if err != nil {
				logger.Levelf(log.Error, "%v", err)
				connErr = err

				return nil
			}

			return conn
		}(),
		FriendlyName:        getDefaultFriendlyName(),
		NoTranscode:         true,
		NoProbe:             true,
		StallEventSubscribe: false,
		Icons:               []dms.Icon{}, // No icons
		LogHeaders:          settings.GetSettings().EnableDebug,
		NotifyInterval:      30 * time.Second,
		AllowedIpNets: func() []*net.IPNet {
			nets := make([]*net.IPNet, 0, 2)
			if _, ipnet, err := net.ParseCIDR("0.0.0.0/0"); err == nil {
				nets = append(nets, ipnet)
			}
			if _, ipnet, err := net.ParseCIDR("::/0"); err == nil {
				nets = append(nets, ipnet)
			}

			return nets
		}(),
		OnBrowseDirectChildren: onBrowse,
		OnBrowseMetadata:       onBrowseMeta,
	}

	if connErr != nil {
		return connErr
	}

	if err := dmsServer.Init(); err != nil {
		return fmt.Errorf("error initing dms server: %w", err)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Levelf(log.Error, "dlna server goroutine panic: %v", r)
			}
		}()

		if err := dmsServer.Run(); err != nil {
			logger.Levelf(log.Error, "%v", err)
		}
	}()

	return nil
}

func Stop() {
	if dmsServer != nil {
		_ = dmsServer.Close()
		dmsServer = nil
	}
}

func onBrowse(path, rootObjectPath, host, userAgent string) (ret []any, err error) {
	if path == "/" {
		ret = getRoot()

		return
	} else if path == "/TR" {
		ret = getTorrents()

		return
	} else if isHashPath(path) {
		ret = getTorrent(path, host)

		return
	} else if filepath.Base(path) == "LD" {
		ret = loadTorrent(path, host)
	}

	return
}

func onBrowseMeta(path string, rootObjectPath string, host, userAgent string) (ret any, err error) {
	ret = getTorrentMeta(path, host)
	if ret == nil {
		err = errors.New("meta not found")
	}

	return
}

// findBestInterfaceForName returns the first suitable network interface
// for constructing a friendly DLNA name. It skips loopback, down,
// and non-multicast interfaces on non-Windows platforms.

// findInterfaceIP returns the first non-loopback IPv4 address of the given
// network interface. It returns an error if no suitable address is found.

// findBestInterfaceForName returns the first suitable network interface
// for constructing a friendly DLNA name. It skips loopback, down,
// and non-multicast interfaces on non-Windows platforms.
//
//nolint:unused // Reserved for future DLNA interface selection
func findBestInterfaceForName(ifaces []net.Interface) *net.Interface {
	for _, i := range ifaces {
		// interface flags seem to always be 0 on Windows
		if runtime.GOOS != "windows" && (i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagMulticast == 0) {
			continue
		}

		return &i
	}

	return nil
}

// findInterfaceIP returns the first non-loopback IPv4 address of the given
// network interface. It returns an error if no suitable address is found.
//
//nolint:unused // Reserved for future DLNA interface selection
func findInterfaceIP(iface *net.Interface) (string, error) {
	if iface == nil {
		return "", errors.New("interface is nil")
	}

	addrs, err := anet.InterfaceAddrsByInterface(iface)
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}

	for _, addr := range addrs {
		var ip net.IP

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}

		if !ip.IsLoopback() && ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return "", errors.New("no suitable IP address found")
}

// collectNonLoopbackIPs gathers all non-loopback IPv4 addresses from the
// provided network interfaces, returning them as a sorted slice.
func collectNonLoopbackIPs(ifaces []net.Interface) []string {
	var ips []string

	for _, i := range ifaces {
		// interface flags seem to always be 0 on Windows
		if runtime.GOOS != "windows" && (i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 || i.Flags&net.FlagMulticast == 0) {
			continue
		}

		addrs, err := anet.InterfaceAddrsByInterface(&i)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if !ip.IsLoopback() && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}

	sort.Strings(ips)

	return ips
}

func getDefaultFriendlyName() string {
	logger := log.Default.WithNames("dlna")

	if settings.GetSettings().FriendlyName != "" {
		return settings.GetSettings().FriendlyName
	}

	userName := ""

	user, err := user.Current()
	if err != nil {
		logger.Printf("getDefaultFriendlyName could not get username: %s", err)
	} else {
		userName = user.Name
	}

	host := ""

	host, err = os.Hostname()
	if err != nil {
		logger.Printf("getDefaultFriendlyName could not get hostname: %s", err)
	}

	if userName == "" && host == "" {
		return "TorrServer"
	}

	if userName != "" && host != "" {
		if userName == host {
			return "TorrServer: " + userName
		}

		return "TorrServer: " + userName + " on " + host
	}

	if host != "localhost" {
		return "TorrServer: " + userName + "@" + host
	}

	// Useless host, use 1st IP
	ifaces, err := anet.Interfaces()
	if err != nil {
		return "TorrServer: " + userName + "@" + host
	}

	ips := collectNonLoopbackIPs(ifaces)
	if len(ips) > 0 {
		return "TorrServer " + ips[0]
	}

	return "TorrServer: " + userName + "@" + host
}
