package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The gateway's health endpoints are only useful if the deployment actually uses
// them: liveness/edge probes keep the cheap /health, while readiness — the signal
// that decides whether a replica receives traffic or may be replaced — must hit
// /health/ready. The drain window after SIGTERM must also be left enough time by
// the orchestrator. This test keeps the four shipped deployment artifacts aligned
// with the code, the same way the nginx timeout test guards the edge budget.
func TestShippedProbesUseReadinessAndAllowTheDrainWindow(t *testing.T) {
	root := filepath.Join("..", "..")

	// Docker Compose: both replicas probe readiness and get a stop grace period.
	compose := readFile(t, filepath.Join(root, "docker-compose.yml"))
	if got := strings.Count(compose, "/health/ready"); got < 2 {
		t.Errorf("docker-compose.yml has %d readiness probe(s), want 2 (one per gateway replica)", got)
	}
	if strings.Contains(compose, `$${GATEWAY_PORT:-8080}/health ||`) {
		t.Error("docker-compose.yml still probes the dependency-free /health endpoint")
	}
	if got := strings.Count(compose, "stop_grace_period:"); got < 2 {
		t.Errorf("docker-compose.yml has %d stop_grace_period setting(s), want 2: Docker's default 10s SIGKILLs the gateway before it can drain", got)
	}
	if !strings.Contains(compose, "GATEWAY_SHUTDOWN_TIMEOUT_SEC") {
		t.Error("docker-compose.yml does not pass GATEWAY_SHUTDOWN_TIMEOUT_SEC")
	}

	// Docker Swarm stack: readiness + stop grace period.
	stack := readFile(t, filepath.Join(root, "docker-stack.yml"))
	if !strings.Contains(stack, "/health/ready") {
		t.Error("docker-stack.yml does not probe /health/ready")
	}
	if !strings.Contains(stack, "stop_grace_period:") {
		t.Error("docker-stack.yml has no stop_grace_period; Swarm's 10s default kills the gateway mid-drain")
	}
	if !strings.Contains(stack, "GATEWAY_SHUTDOWN_TIMEOUT_SEC") {
		t.Error("docker-stack.yml does not pass GATEWAY_SHUTDOWN_TIMEOUT_SEC")
	}

	// Helm chart: readiness probe on /health/ready, liveness on /health, and
	// terminationGracePeriodSeconds above the shutdown budget.
	values := struct {
		ShutdownTimeoutSeconds        int `yaml:"shutdownTimeoutSeconds"`
		TerminationGracePeriodSeconds int `yaml:"terminationGracePeriodSeconds"`
	}{}
	if err := yaml.Unmarshal([]byte(readFile(t, filepath.Join(root, "helm", "airoute", "values.yaml"))), &values); err != nil {
		t.Fatalf("unmarshal helm values: %v", err)
	}
	if values.ShutdownTimeoutSeconds <= 0 {
		t.Error("helm values: shutdownTimeoutSeconds must be set")
	}
	if values.TerminationGracePeriodSeconds <= values.ShutdownTimeoutSeconds {
		t.Errorf("helm values: terminationGracePeriodSeconds (%d) must exceed shutdownTimeoutSeconds (%d), otherwise kubelet SIGKILLs the process mid-drain",
			values.TerminationGracePeriodSeconds, values.ShutdownTimeoutSeconds)
	}
	deployment := readFile(t, filepath.Join(root, "helm", "airoute", "templates", "deployment.yaml"))
	if !containsAll(deployment, "readinessProbe:", "/health/ready") {
		t.Error("helm deployment readinessProbe does not use /health/ready")
	}
	if !strings.Contains(deployment, "terminationGracePeriodSeconds: {{") {
		t.Error("helm deployment has no configurable terminationGracePeriodSeconds")
	}
	if !strings.Contains(deployment, "GATEWAY_SHUTDOWN_TIMEOUT_SEC") {
		t.Error("helm deployment does not pass GATEWAY_SHUTDOWN_TIMEOUT_SEC")
	}

	// Static Kubernetes manifest shipped in deploy/ must agree too.
	k8s := readFile(t, filepath.Join(root, "deploy", "kubernetes.yaml"))
	if !containsAll(k8s, "readinessProbe:", "/health/ready") {
		t.Error("deploy/kubernetes.yaml readinessProbe does not use /health/ready")
	}
	if !strings.Contains(k8s, "terminationGracePeriodSeconds:") {
		t.Error("deploy/kubernetes.yaml has no terminationGracePeriodSeconds")
	}

	// The rolling rebuild script still gates on the container health status; with
	// the healthchecks above that is now the readiness signal. Guard the poll.
	rebuild := readFile(t, filepath.Join(root, "scripts", "docker-rebuild.sh"))
	if !strings.Contains(rebuild, ".State.Health.Status") {
		t.Error("scripts/docker-rebuild.sh no longer waits for the container health status; rollouts could replace a replica that is not ready")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
