package middleware

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Trusted-proxy parsing and client-origin resolution, extracted from authz.go to
// keep the denial/admission file under the size red line.

func ParseTrustedProxies(spec string) ([]*net.IPNet, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	nets := make([]*net.IPNet, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(field); err == nil {
			nets = append(nets, network)
			continue
		}
		ip := net.ParseIP(field)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy address %q (want an IP or CIDR)", field)
		}
		if v4 := ip.To4(); v4 != nil {
			nets = append(nets, &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)})
		} else {
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	return nets, nil
}

// TrustedProxiesFromEnv returns the configured trusted proxy networks, caching
// the parse per environment value so it can run on every request.
func TrustedProxiesFromEnv() ([]*net.IPNet, error) {
	spec := strings.TrimSpace(os.Getenv(trustedProxyEnv))
	if cached := trustedProxyCache.Load(); cached != nil && cached.spec == spec {
		return cached.nets, cached.err
	}
	nets, err := ParseTrustedProxies(spec)
	trustedProxyCache.Store(&trustedProxyCacheEntry{spec: spec, nets: nets, err: err})
	return nets, err
}

// PeerIsTrustedProxy reports whether the request's direct TCP peer is one of the
// configured trusted reverse proxies. Forwarding headers may only be believed when
// this is true: an untrusted peer can write any value it likes into them.
//
// It mirrors the peer check inside ResolveClientIdentity; callers that need the
// *scheme* rather than the address (cookie Secure flags, redirect building) use it
// directly.
func PeerIsTrustedProxy(r *http.Request) bool {
	if r == nil {
		return false
	}
	peer := peerIP(r.RemoteAddr)
	if peer == "" {
		return false
	}
	trusted, err := TrustedProxiesFromEnv()
	if err != nil {
		return false
	}
	return ipInNetworks(net.ParseIP(peer), trusted)
}

// TrustedForwardedProto returns the X-Forwarded-Proto value only when the request
// arrived from a configured trusted proxy. Any other request reports ("", false),
// because the header is then the caller's own claim rather than the proxy's
// observation of the scheme it terminated.
func TrustedForwardedProto(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil {
		return "", false
	}
	if !PeerIsTrustedProxy(c.Request) {
		return "", false
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	if proto == "" {
		return "", false
	}
	return proto, true
}

// TrustedProxyConfigured reports whether any reverse proxy is trusted. Without
// one, forwarding headers are ignored (there is no way to tell a real client
// address from a forged one).
func TrustedProxyConfigured() bool {
	nets, err := TrustedProxiesFromEnv()
	return err == nil && len(nets) > 0
}

// ConfigureTrustedProxies applies GATEWAY_TRUSTED_PROXIES to the Gin engine so
// every c.ClientIP() caller sees the same, unspoofable address as
// ResolveClientIdentity. An invalid list is an error rather than a silent
// fallback, because "trust everything" is exactly the failure being fixed.
func ConfigureTrustedProxies(engine *gin.Engine) error {
	if engine == nil {
		return nil
	}
	spec := strings.TrimSpace(os.Getenv(trustedProxyEnv))
	nets, err := ParseTrustedProxies(spec)
	if err != nil {
		return err
	}
	if len(nets) == 0 {
		// Setting an empty list (nil) makes Gin ignore forwarding headers and
		// report the TCP peer.
		return engine.SetTrustedProxies(nil)
	}
	proxies := make([]string, 0, len(nets))
	for _, network := range nets {
		proxies = append(proxies, network.String())
	}
	return engine.SetTrustedProxies(proxies)
}

// ResolveClientIdentity resolves the request origin, honouring forwarding
// headers only when the direct peer is a configured trusted proxy and the
// forwarded address itself is not another trusted proxy.
func ResolveClientIdentity(c *gin.Context) ClientIdentity {
	if c == nil || c.Request == nil {
		return ClientIdentity{}
	}
	peer := peerIP(c.Request.RemoteAddr)
	if peer == "" {
		return ClientIdentity{}
	}
	trusted, err := TrustedProxiesFromEnv()
	peerIPValue := net.ParseIP(peer)
	if err != nil || !ipInNetworks(peerIPValue, trusted) {
		// Untrusted (or unconfigured) peer: any forwarding header it sends is
		// self-asserted, so the peer is the only address we can attribute.
		return ClientIdentity{IP: peer}
	}

	// Walk X-Forwarded-For from the right: the last trusted proxy appends the
	// last untrusted hop, which is the closest thing to the real client.
	if forwarded := c.Request.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		hops := strings.Split(strings.Join(forwarded, ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(hops[i]))
			if ip == nil {
				break
			}
			if !ipInNetworks(ip, trusted) {
				return ClientIdentity{IP: ip.String(), Forwarded: true}
			}
		}
		// Every hop was itself a trusted proxy; no client attribution exists.
		return ClientIdentity{IP: peer}
	}

	// X-Real-IP is the second header the shipped nginx sets.
	if realIP := net.ParseIP(strings.TrimSpace(c.GetHeader("X-Real-IP"))); realIP != nil && !ipInNetworks(realIP, trusted) {
		return ClientIdentity{IP: realIP.String(), Forwarded: true}
	}
	return ClientIdentity{IP: peer}
}

// ResolveClientIP is ResolveClientIdentity(...).IP.
func ResolveClientIP(c *gin.Context) string {
	return ResolveClientIdentity(c).IP
}

func peerIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return remoteAddr
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, network := range networks {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}