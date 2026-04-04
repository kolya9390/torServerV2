package startup

import (
	"fmt"
	"net"
	"strconv"

	"server/settings"
)

const (
	defaultHTTPPort  = "8090"
	defaultHTTPSPort = "8091"
)

var listenTCP = net.Listen

// PrepareNetwork validates web network settings and resolves final runtime ports.
// It mutates args/BT settings with defaults for compatibility with existing flow.
func PrepareNetwork(args *settings.ExecArgs) error {
	if args == nil {
		return fmt.Errorf("exec args are not initialized")
	}
	if args.Ssl {
		if err := prepareSSL(args); err != nil {
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

func prepareSSL(args *settings.ExecArgs) error {
	if args.SslPort == "" {
		dbSSLPort := strconv.Itoa(settings.BTsets.SslPort)
		if dbSSLPort != "0" {
			args.SslPort = dbSSLPort
		} else {
			args.SslPort = defaultHTTPSPort
		}
	} else {
		dbSSLPort, err := strconv.Atoi(args.SslPort)
		if err == nil {
			settings.BTsets.SslPort = dbSSLPort
		}
	}
	if err := ensurePortFree(args.IP, args.SslPort, "ssl"); err != nil {
		return err
	}
	return nil
}

func ensurePortFree(ip, port, label string) error {
	l, err := listenTCP("tcp", ip+":"+port)
	if l != nil {
		_ = l.Close()
	}
	if err != nil {
		return fmt.Errorf("%s port %s already in use", label, port)
	}
	return nil
}
