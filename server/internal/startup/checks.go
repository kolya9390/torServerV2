package startup

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"

	"server/settings"
)

const (
	defaultHTTPPort  = "8090"
	defaultHTTPSPort = "8091"
)

var listenTCP = net.Listen

// PrepareNetwork validates web network settings and resolves final runtime ports.
// It mutates args/BT settings with defaults for compatibility with existing flow.
// PrepareNetwork is a legacy compatibility wrapper; production composition should call PrepareNetworkWithProvider.
func PrepareNetwork(args *settings.ExecArgs) error {
	return PrepareNetworkWithProvider(args, settings.DefaultSettingsProvider)
}

func PrepareNetworkWithProvider(args *settings.ExecArgs, provider settings.SettingsProvider) error {
	if args == nil {
		return errors.New("exec args are not initialized")
	}

	if args.Ssl {
		if err := prepareSSL(args, provider); err != nil {
			return err
		}
	}

	if args.Port == "" {
		args.Port = defaultHTTPPort
	}

	if err := ensurePortFree(args.IP, args.Port, "http"); err != nil {
		return err
	}

	return nil
}

func prepareSSL(args *settings.ExecArgs, provider settings.SettingsProvider) error {
	if provider == nil {
		provider = settings.NewNoopSettingsProvider()
	}

	curSets := provider.Get()
	curTLS := curSets.TLSConfig()

	if args.SslPort == "" {
		dbSSLPort := strconv.Itoa(curTLS.Port)
		if dbSSLPort != "0" {
			args.SslPort = dbSSLPort
		} else {
			args.SslPort = defaultHTTPSPort
		}
	} else {
		dbSSLPort, err := strconv.Atoi(args.SslPort)
		if err == nil {
			curSets.SslPort = dbSSLPort
		}
	}

	if err := ensurePortFree(args.IP, args.SslPort, "ssl"); err != nil {
		return err
	}

	return nil
}

func ensurePortFree(ip, port, label string) error {
	address := net.JoinHostPort(ip, port)
	l, err := listenTCP("tcp", address)
	if l != nil {
		if closeErr := l.Close(); closeErr != nil {
			return fmt.Errorf("close %s port %s probe listener: %w", label, port, closeErr)
		}
	}

	if err != nil {
		if isAddressAlreadyInUse(err) {
			return fmt.Errorf("%s port %s already in use: %w", label, port, err)
		}

		return fmt.Errorf("probe %s listener on %s: %w", label, address, err)
	}

	return nil
}

func isAddressAlreadyInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || strings.Contains(strings.ToLower(err.Error()), "address already in use")
}
