package router

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestRoutingSnapshotUsesModelPriorityAndActualBreakerState(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "unrelated", Type: model.ProviderGPUStack, Models: []string{"other"}, Priority: 1},
		{ID: 2, Name: "backup", Type: model.ProviderOpenAI, Models: []string{"alias"}, Priority: 8, Weight: 4},
		{ID: 3, Name: "primary", Type: model.ProviderGPUStack, Models: []string{"alias"}, ModelMapping: map[string]string{"alias": "real-model"}, Priority: 2, Weight: 10, APIKey: "never-expose", BaseURL: "https://private.example?key=secret"},
		{ID: 4, Name: "fallback", Type: model.ProviderOpenAI, Models: []string{"dr"}, Priority: 7, Weight: 2},
	})
	d.SetModelFallbacks(map[string]string{"alias": "dr"})
	for i := 0; i < 3; i++ {
		d.circuitBreaker.RecordFailure("primary")
	}
	for i := 0; i < 5; i++ {
		snapshot := d.GetRoutingSnapshot("alias")
		if len(snapshot.Steps) != 2 || len(snapshot.Steps[0].Providers) != 2 {
			t.Fatalf("wrong model graph: %+v", snapshot)
		}
		primary, backup := snapshot.Steps[0].Providers[0], snapshot.Steps[0].Providers[1]
		if primary.Name != "primary" || primary.Priority != 2 || primary.MappedModel != "real-model" || primary.Eligible || primary.Breaker.State != "OPEN" || !backup.Eligible {
			t.Fatalf("wrong runtime state: %+v", snapshot.Steps[0])
		}
		body, _ := json.Marshal(snapshot)
		if strings.Contains(string(body), "never-expose") || strings.Contains(string(body), "private.example") {
			t.Fatal("routing snapshot exposed upstream credentials or URL")
		}
	}
	if d.GetBreakerStatus("primary") != "OPEN" {
		t.Fatal("inspection mutated breaker")
	}
	d.circuitBreaker.mu.Lock()
	d.circuitBreaker.breakers["primary"].LastStateChange = time.Now().Add(-time.Hour)
	d.circuitBreaker.mu.Unlock()
	probe := d.GetRoutingSnapshot("alias").Steps[0].Providers[0]
	if !probe.Eligible || !probe.Breaker.RecoveryProbe || d.GetBreakerStatus("primary") != "OPEN" {
		t.Fatal("read-only recovery eligibility changed admission state")
	}
}

func TestRoutingSnapshotReportsCyclesMissingTargetsAndEmptyState(t *testing.T) {
	d := NewDispatcher(nil)
	if got := d.GetRoutingSnapshot(""); len(got.Steps) != 0 || len(got.Models) != 0 {
		t.Fatalf("invented empty routing graph: %+v", got)
	}
	d.SetModelFallbacks(map[string]string{"a": "b", "b": "a"})
	got := d.GetRoutingSnapshot("a")
	if got.ChainError == "" || len(got.Steps) != 2 || len(got.Steps[1].Providers) != 0 {
		t.Fatalf("cycle or absent target hidden: %+v", got)
	}
}
