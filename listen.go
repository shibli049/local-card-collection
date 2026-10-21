package main

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

func normalizeListenHost(host string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1":
		return "127.0.0.1", nil
	case "0.0.0.0":
		return "0.0.0.0", nil
	default:
		return "", errors.New("choose --host 127.0.0.1 for this computer only, or --host 0.0.0.0 for network access")
	}
}

func listenHTTP(host string, port int) (net.Listener, error) {
	host, err := normalizeListenHost(host)
	if err != nil {
		return nil, err
	}
	if port < 0 || port > 65535 {
		return nil, errors.New("choose a port between 1 and 65535, or use --port 0 to choose an available port")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil && port == 8765 {
		listener, err = net.Listen("tcp4", net.JoinHostPort(host, "0"))
	}
	return listener, err
}

func allowedRequestHost(authority string, networkAccess bool) bool {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		host = authority
	}
	if host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost") {
		return true
	}
	// Network mode accepts IP-address URLs, not arbitrary domain names. Keep
	// this host check as well as the existing origin and cross-site checks.
	return networkAccess && net.ParseIP(host) != nil
}
