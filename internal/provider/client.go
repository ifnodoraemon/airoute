package provider

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// SharedDefaultHTTPClient is a globally tuned, pre-warmed HTTP client designed for
// extreme throughput and low latency. It reuses TCP/TLS connections across thousands of
// concurrent upstream requests to prevent socket exhaustion and latency spikes.
var SharedDefaultHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2048,
		MaxIdleConnsPerHost:   512,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	},
	Timeout: 0, // No client-level body deadline to allow long-running SSE streams; ResponseHeaderTimeout protects headers
}
