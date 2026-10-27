package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"gopkg.in/yaml.v3"
)

// The proxy budget exists in several places: the model constant that bounds
// channel timeouts, and every edge proxy config this repository ships. They must
// all be the same number, and this test is what keeps them the same - otherwise
// an operator raises a channel timeout and one of the shipped proxies silently
// truncates it again. The previous version of this test only read the Compose
// config, so the Swarm config (600s) and the Helm ingress (300s) kept truncating
// long streams while the test stayed green.
var edgeTimeoutDirective = regexp.MustCompile(`(?m)^[ \t]*(proxy_read_timeout|proxy_send_timeout)[ \t]+(\d+)s[ \t]*;`)

// shippedNginxConfigs lists every nginx config shipped by this repository.
var shippedNginxConfigs = []string{
	filepath.Join("..", "..", "deploy", "nginx-compose.conf"),        // Docker Compose edge
	filepath.Join("..", "..", "deploy", "swarm", "nginx-swarm.conf"), // Docker Swarm edge
	filepath.Join("..", "..", "deploy", "nginx.conf"),                // single-host / legacy edge
}

// TestShippedNginxTimeoutsMatchModelConstant fails if any shipped nginx config
// declares a proxy timeout that differs from the model's declared edge budget.
func TestShippedNginxTimeoutsMatchModelConstant(t *testing.T) {
	want := model.EdgeProxyTimeoutSeconds

	for _, path := range shippedNginxConfigs {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			matches := edgeTimeoutDirective.FindAllStringSubmatch(string(raw), -1)
			if len(matches) == 0 {
				t.Fatalf("no proxy_read_timeout/proxy_send_timeout directive found in %s; "+
					"the edge proxy must declare the budget explicitly", path)
			}

			seen := make(map[string]bool, 2)
			for _, match := range matches {
				directive := match[1]
				seconds, err := strconv.Atoi(match[2])
				if err != nil {
					t.Fatalf("unparsable %s value %q in %s", directive, match[2], path)
				}
				seen[directive] = true
				if seconds != want {
					t.Errorf("%s = %ds in %s, but the model declares the proxy budget as %ds: "+
						"the proxy must never be the component that ends a request",
						directive, seconds, path, want)
				}
			}
			for _, directive := range []string{"proxy_read_timeout", "proxy_send_timeout"} {
				if !seen[directive] {
					t.Errorf("%s is not declared in %s; both read and send timeouts must be set", directive, path)
				}
			}
		})
	}
}

// TestShippedHelmIngressTimeoutsMatchModelConstant covers the Kubernetes edge,
// which is not an nginx.conf file: the Ingress controller timeouts are chart
// values, so they need the same invariant check. Without the annotation the
// controller falls back to its own default (60s) and truncates streams again.
func TestShippedHelmIngressTimeoutsMatchModelConstant(t *testing.T) {
	path := filepath.Join("..", "..", "helm", "airoute", "values.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var values struct {
		Ingress struct {
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"ingress"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}

	want := strconv.Itoa(model.EdgeProxyTimeoutSeconds)
	for _, key := range []string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout",
		"nginx.ingress.kubernetes.io/proxy-send-timeout",
	} {
		got, ok := values.Ingress.Annotations[key]
		if !ok {
			t.Fatalf("%s is missing from ingress.annotations in %s; "+
				"without it the ingress controller falls back to its 60s default and truncates streams", key, path)
		}
		if got != want {
			t.Errorf("%s = %q in %s, but the model declares the proxy budget as %ss: "+
				"the ingress must never be the component that ends a request", key, got, path, want)
		}
	}
}
