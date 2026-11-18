package service

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")
var translationPrefix = netip.MustParsePrefix("64:ff9b::/96")
var transitionPrefix = netip.MustParsePrefix("2002::/16")

func publicWebhookAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !sharedAddressSpace.Contains(ip) && !translationPrefix.Contains(ip) && !transitionPrefix.Contains(ip)
}

func allowedWebhookHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		return publicWebhookAddress(ip)
	}
	return strings.Contains(host, ".") && !strings.HasSuffix(host, ".localhost") && !strings.HasSuffix(host, ".local")
}

func dialWebhook(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if !allowedWebhookHost(host) {
		return nil, fmt.Errorf("đích webhook không công khai")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range addresses {
		if !publicWebhookAddress(ip) {
			return nil, fmt.Errorf("đích webhook không công khai")
		}
	}
	dialer := net.Dialer{}
	for _, ip := range addresses {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	return nil, fmt.Errorf("không kết nối được đích webhook")
}
