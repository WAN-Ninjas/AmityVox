// Package webhooks — SSRF protection for outgoing webhook delivery.
package webhooks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// isPrivateIP returns true if the IP is in a private, loopback, link-local,
// or otherwise non-public range. Used to prevent SSRF attacks in outgoing
// webhook delivery.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// safeTransport returns an http.Transport that validates resolved IPs at
// connection time, preventing DNS rebinding attacks. Any attempt to connect
// to a private/loopback address is rejected.
func safeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", addr, err)
			}

			// Resolve the hostname to IPs.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS resolution failed for %q: %w", host, err)
			}

			// Validate ALL resolved IPs — reject if any are private.
			for _, ipAddr := range ips {
				if isPrivateIP(ipAddr.IP) {
					return nil, fmt.Errorf("webhook URL resolves to private address %s", ipAddr.IP)
				}
			}

			// Connect using the first valid IP.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxIdleConns:           10,
		IdleConnTimeout:        30 * time.Second,
	}
}

// safeHTTPClient returns an http.Client with SSRF-safe transport.
func safeHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: safeTransport(),
	}
}
