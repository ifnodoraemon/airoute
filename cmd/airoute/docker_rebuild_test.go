package main

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerRebuildCoordinatesOnlyNeededStorageAndAlwaysReconcilesEdge(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "deploy/nginx-compose.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, driver, endpoint string
		wantStorage, drift     bool
	}{
		{"local", "local", "http://storage:9000", false, false},
		{"external_s3", "s3", "https://objects.example.invalid", false, false},
		{"managed_s3", "s3", "http://storage:9000", true, false},
		{"managed_rustfs", "rustfs", "https://storage:9000", true, false},
		{"changed_bind_inode", "local", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			calls := filepath.Join(dir, "calls")
			docker := `#!/bin/sh
printf '%s\n' "$*" >> "$PROBE_CALLS"
case "$1 $2" in
    'compose config')
        printf '{\n "STORAGE_DRIVER": "%s",\n "STORAGE_S3_ENDPOINT": "%s"\n}\n' "$PROBE_DRIVER" "$PROBE_ENDPOINT" ;;
    'inspect -f') printf 'healthy\n' ;;
    'exec airoute-lb')
        if [ "$3" = md5sum ]; then printf '%s  config\n' "$PROBE_HASH"; fi ;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(docker), 0755); err != nil {
				t.Fatal(err)
			}
			hash := fmt.Sprintf("%x", md5.Sum(config))
			if tc.drift {
				hash = "old-file-inode"
			}
			command := exec.Command("sh", filepath.Join(root, "scripts/docker-rebuild.sh"))
			command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "VERSION=review-test", "PROBE_CALLS="+calls,
				"PROBE_DRIVER="+tc.driver, "PROBE_ENDPOINT="+tc.endpoint, "PROBE_HASH="+hash, "GATEWAY_INGRESS_PORT=18081")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("rebuild failed: %v\n%s", err, output)
			}
			output, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			text := string(output)
			if got := strings.Contains(text, "compose up -d --no-deps postgres redis storage"); got != tc.wantStorage {
				t.Fatalf("storage requested=%t want=%t\n%s", got, tc.wantStorage, text)
			}
			if !strings.Contains(text, "compose up -d --no-deps lb\n") || !strings.Contains(text, "nginx -s reload") {
				t.Fatalf("edge settings not reconciled: %s", text)
			}
			if strings.Contains(text, "--force-recreate lb") != tc.drift {
				t.Fatalf("bind-file drift handling incorrect: %s", text)
			}
		})
	}
}
