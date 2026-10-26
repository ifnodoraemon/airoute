package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// The proxy budget exists in two places: the model constant that bounds channel
// timeouts, and the shipped nginx config that enforces it at the edge. They must be
// the same number, and this test is what keeps them the same - otherwise an operator
// raises a channel timeout and the proxy silently truncates it again.
func TestShippedNginxTimeoutMatchesModelConstant(t *testing.T) {
	path := filepath.Join("..", "..", "deploy", "nginx-compose.conf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	for _, directive := range []string{"proxy_read_timeout", "proxy_send_timeout"} {
		pattern := regexp.MustCompile(directive + `\s+(\d+)s;`)
		matches := pattern.FindAllStringSubmatch(string(raw), -1)
		if len(matches) == 0 {
			t.Fatalf("no %s directive found in %s", directive, path)
		}
		for _, match := range matches {
			seconds, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatalf("unparsable %s value %q", directive, match[1])
			}
			if seconds != model.EdgeProxyTimeoutSeconds {
				t.Fatalf("%s = %ds in %s, but the model declares the proxy budget as %ds: "+
					"the proxy must never be the component that ends a request",
					directive, seconds, path, model.EdgeProxyTimeoutSeconds)
			}
		}
	}
}
