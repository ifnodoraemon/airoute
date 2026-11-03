package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// controlPlaneLocation is the location directive the shipped edge configs use for
// the management surface. `location ~*` (the api.* vhost's blanket 403) does not
// match this marker on purpose: those vhosts deny the path outright and do not
// need an allowlist.
const controlPlaneLocation = "location ~ ^/(api/v1/admin|app)(/|$) {"

// controlPlaneAllows are the networks the edge must keep reachable: every shipped
// topology only exposes the gateway on private networks.
var controlPlaneAllows = []string{
	"allow 127.0.0.1;",
	"allow 10.0.0.0/8;",
	"allow 172.16.0.0/12;",
	"allow 192.168.0.0/16;",
}

// nginxBraceDelta counts the braces that open/close a config block. JSON error
// bodies are single-quoted nginx strings and contain braces of their own; counting
// them would end a server block in the middle of an error_page directive.
func nginxBraceDelta(line string) int {
	var braces strings.Builder
	inQuote := false
	for _, r := range line {
		switch r {
		case '\'':
			inQuote = !inQuote
		case '{', '}':
			if !inQuote {
				braces.WriteRune(r)
			}
		}
	}
	counted := braces.String()
	return strings.Count(counted, "{") - strings.Count(counted, "}")
}

// nginxBlockStartingAt returns the brace-balanced block that starts on the first
// line accepted by lineMatch.
func nginxBlockStartingAt(raw string, lineMatch func(string) bool) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !lineMatch(line) {
			continue
		}
		var block strings.Builder
		depth := 0
		for _, cur := range lines[i:] {
			block.WriteString(cur)
			block.WriteString("\n")
			depth += nginxBraceDelta(cur)
			if depth <= 0 {
				break
			}
		}
		return block.String()
	}
	return ""
}

// nginxBlocksContaining returns every block that starts on a matching line and
// contains needle.
func nginxBlocksContaining(raw string, lineMatch func(string) bool, needle string) []string {
	var blocks []string
	rest := raw
	for {
		block := nginxBlockStartingAt(rest, lineMatch)
		if block == "" {
			return blocks
		}
		if strings.Contains(block, needle) {
			blocks = append(blocks, block)
		}
		rest = rest[strings.Index(rest, block)+len(block):]
	}
}

func isControlPlaneLocationLine(line string) bool {
	return strings.Contains(line, controlPlaneLocation)
}

func isServerBlockStart(line string) bool {
	return strings.TrimSpace(line) == "server {"
}

// The edge ACL is what stops a public client from reaching the control plane when
// the gateway's own guard is misconfigured, and it only works if every control-plane
// location in every shipped config applies it. It used to exist on one vhost of one
// of the three configs, so `Host: console.<anything>` (a client-chosen header)
// reached /api/v1/admin with no edge check, and the Swarm/single-host files had no
// ACL at all. This is the manifest half of A-01/A-02, the same way
// nginx_timeout_test.go guards the timeout budget.
func TestShippedNginxConfigsApplyControlPlaneACL(t *testing.T) {
	for _, path := range shippedNginxConfigs {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw := readFile(t, path)
			blocks := nginxBlocksContaining(raw, isControlPlaneLocationLine, controlPlaneLocation)
			if len(blocks) == 0 {
				t.Fatalf("%s has no %q location: the edge must restrict /api/v1/admin and /app", path, controlPlaneLocation)
			}
			for i, block := range blocks {
				if !strings.Contains(block, "deny all;") {
					t.Errorf("%s control-plane location #%d has no `deny all;`, so it forwards every client", path, i+1)
				}
				for _, allow := range controlPlaneAllows {
					if !strings.Contains(block, allow) {
						t.Errorf("%s control-plane location #%d is missing %q; loopback/LAN operators would be locked out", path, i+1, allow)
					}
				}
			}
		})
	}

	// The Host-based split is a routing convenience, not a boundary: any client can
	// send `Host: console.<anything>` and land in the dedicated control-plane vhost.
	// That vhost therefore needs the same ACL, or it becomes a bypass.
	for _, path := range []string{
		filepath.Join("..", "..", "deploy", "nginx-compose.conf"),
		filepath.Join("..", "..", "deploy", "swarm", "nginx-swarm.conf"),
	} {
		raw := readFile(t, path)
		vhosts := nginxBlocksContaining(raw, isServerBlockStart, "server_name console.")
		if len(vhosts) == 0 {
			t.Fatalf("%s has no console/admin vhost to check", path)
		}
		for i, vhost := range vhosts {
			if !strings.Contains(vhost, controlPlaneLocation) {
				t.Errorf("%s console/admin vhost #%d has no %q location: a client-supplied Host header bypasses the edge ACL", path, i+1, controlPlaneLocation)
			}
		}
	}
}

// The 413 body is the only hint a caller gets about which component rejected the
// payload, so it must name the edge proxy (the limit it enforces) and that limit
// must not be smaller than the gateway's own GATEWAY_MAX_BODY_BYTES default.
func TestShippedNginxBodyLimitMatchesGateway(t *testing.T) {
	const defaultGatewayBodyBytes = 64 << 20 // internal/middleware/body_limit.go
	for _, path := range shippedNginxConfigs {
		raw := readFile(t, path)
		if !strings.Contains(raw, "client_max_body_size 64m;") {
			t.Errorf("%s does not allow 64m request bodies, but the gateway accepts %d bytes; the edge would reject payloads the gateway handles", path, defaultGatewayBodyBytes)
		}
		if strings.Contains(raw, "gateway limit") {
			t.Errorf("%s still blames the gateway in its 413 body; the rejection happens at the edge proxy", path)
		}
	}
}
