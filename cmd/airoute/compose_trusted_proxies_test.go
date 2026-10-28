package main

import (
	"net"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeDoc is the slice of docker-compose.yml this test reasons about.
type composeDoc struct {
	Services map[string]struct {
		Environment map[string]string `yaml:"environment"`
		Networks    yaml.Node         `yaml:"networks"`
	} `yaml:"services"`
	Networks map[string]struct {
		IPAM struct {
			Config []struct {
				Subnet string `yaml:"subnet"`
			} `yaml:"config"`
		} `yaml:"ipam"`
	} `yaml:"networks"`
}

// TestShippedComposeTrustsOnlyTheProxyAddress pins the invariant behind the
// control-plane guard's client attribution in the shipped Compose topology.
//
// 172.30.0.0/16 in GATEWAY_TRUSTED_PROXIES looks harmless - it is the nginx↔
// gateway network - but traffic published on the host port reaches nginx with the
// bridge gateway's address (the subnet's .1) as its source. Trusting the whole
// range therefore reads that address as another trusted proxy hop, leaves the
// client unattributable, and makes /app and /api/v1/admin return 403 for the
// host's own browser (measured live: every control-plane request from the host was
// denied while data-plane traffic worked).
//
// The fix is to pin the load balancer to a fixed address and trust exactly that
// /32. This test keeps the four moving parts aligned: the fixed subnet, the pinned
// LB address, the Compose default, and the /32 (single-address) shape.
func TestShippedComposeTrustsOnlyTheProxyAddress(t *testing.T) {
	raw := readFile(t, filepath.Join("..", "..", "docker-compose.yml"))
	var doc composeDoc
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}

	edge, ok := doc.Networks["edge"]
	if !ok {
		t.Fatal("docker-compose.yml has no `edge` network")
	}
	if len(edge.IPAM.Config) == 0 || strings.TrimSpace(edge.IPAM.Config[0].Subnet) == "" {
		t.Fatal("the `edge` network must keep a fixed subnet so the trusted-proxy address is stable")
	}
	subnet := strings.TrimSpace(edge.IPAM.Config[0].Subnet)
	_, edgeNet, err := net.ParseCIDR(subnet)
	if err != nil {
		t.Fatalf("edge subnet %q: %v", subnet, err)
	}

	// 1. The load balancer must hold a fixed address inside that subnet.
	lb, ok := doc.Services["lb"]
	if !ok {
		t.Fatal("docker-compose.yml has no `lb` service")
	}
	var lbNetworks map[string]struct {
		IPv4Address string `yaml:"ipv4_address"`
	}
	if err := lb.Networks.Decode(&lbNetworks); err != nil {
		t.Fatalf("decode lb networks (expected a mapping with ipv4_address): %v", err)
	}
	lbAddr := strings.TrimSpace(lbNetworks["edge"].IPv4Address)
	if lbAddr == "" {
		t.Fatal("lb must pin a static ipv4_address on the edge network; a dynamic address cannot be named in GATEWAY_TRUSTED_PROXIES")
	}
	lbIP := net.ParseIP(lbAddr)
	if lbIP == nil || !edgeNet.Contains(lbIP) {
		t.Fatalf("lb address %q is not a usable address inside the edge subnet %s", lbAddr, subnet)
	}

	// 2. The shipped default must name exactly that address, as a single host.
	gateway, ok := doc.Services["gateway-1"]
	if !ok {
		t.Fatal("docker-compose.yml has no `gateway-1` service")
	}
	rawTrusted := strings.TrimSpace(gateway.Environment["GATEWAY_TRUSTED_PROXIES"])
	if rawTrusted == "" {
		t.Fatal("gateway-1 does not set GATEWAY_TRUSTED_PROXIES; behind the shipped proxy the control plane would fail closed for everyone")
	}
	// The value is an env expansion with a default: ${VAR:-default}.
	trusted := rawTrusted
	if i := strings.Index(trusted, ":-"); i >= 0 {
		trusted = strings.TrimSuffix(trusted[i+2:], "}")
	}
	trusted = strings.TrimSpace(trusted)
	if want := lbAddr + "/32"; trusted != want {
		t.Fatalf("GATEWAY_TRUSTED_PROXIES default is %q, want %q (the load balancer's own address)", trusted, want)
	}

	// 3. It must be a single address: a range is what caused the live 403.
	_, trustedNet, err := net.ParseCIDR(trusted)
	if err != nil {
		t.Fatalf("trusted proxy value %q is not a CIDR: %v", trusted, err)
	}
	if ones, bits := trustedNet.Mask.Size(); ones != bits {
		t.Fatalf("GATEWAY_TRUSTED_PROXIES must be a /32 (one proxy address), got prefix /%d: a wider range also covers the host-port source address", ones)
	}

	// 4. Explicitly: the address Docker uses as the source of host-published
	// traffic (the subnet's first host) must not be trusted.
	hostPortSource := make(net.IP, len(edgeNet.IP))
	copy(hostPortSource, edgeNet.IP)
	hostPortSource[len(hostPortSource)-1]++
	if trustedNet.Contains(hostPortSource) {
		t.Fatalf("GATEWAY_TRUSTED_PROXIES %s covers %s, the source address of traffic published on the host port; "+
			"the gateway would read it as a proxy hop and return 403 for host-local /app and /api/v1/admin", trusted, hostPortSource)
	}
}
