package controlplane

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestAPIKeyRejectsUnconfiguredPricingGroup(t *testing.T) {
	handler, repo := newChannelTestHandler(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}
	key := &storage.APIKeyRecord{Key: "sk-group-validation-synthetic", TenantID: "test", GroupName: "default"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			method, endpoint, id := http.MethodPost, handler.CreateAPIKey, ""
			if operation == "update" {
				method, endpoint, id = http.MethodPut, handler.UpdateAPIKey, int64String(key.ID)
			}
			w := invokeKeyWithClaims(t, endpoint, admin, method, `{"group_name":"not-a-configured-group"}`, id)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "请选择已有分组") {
				t.Fatalf("unknown group accepted: status=%d body=%s", w.Code, w.Body.String())
			}
			keys, err := repo.ListAPIKeys()
			if err != nil || len(keys) != 1 || keys[0].GroupName != "default" {
				t.Fatalf("rejected request changed keys: keys=%v err=%v", keys, err)
			}
		})
	}
}

func TestAPIKeySelectsExistingPricingGroups(t *testing.T) {
	handler, repo := newChannelTestHandler(t)
	previous := billing.GlobalEngine
	billing.GlobalEngine = billing.NewEngine(repo)
	t.Cleanup(func() { billing.GlobalEngine = previous })
	if err := billing.GlobalEngine.SetGroupMultipliers(map[string]float64{"default": 1, "tier-only": 0.9}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{Model: "test", GroupName: "rate-only", PromptPrice: 1, Currency: "CNY"}); err != nil {
		t.Fatal(err)
	}
	admin := &AdminClaims{Username: "admin", Role: "admin"}
	for _, group := range []string{"", "default", "tier-only", "rate-only", " RATE-ONLY "} {
		t.Run(group, func(t *testing.T) {
			body := fmt.Sprintf(`{"group_name":%q}`, group)
			w := invokeKeyWithClaims(t, handler.CreateAPIKey, admin, http.MethodPost, body, "")
			if w.Code != http.StatusOK {
				t.Fatalf("existing group rejected: status=%d body=%s", w.Code, w.Body.String())
			}
			data := decodeEnvelope(t, w)["data"].(map[string]any)
			if data["group_name"] != normalizeKeyGroup(group) {
				t.Fatalf("group_name=%v, want %q", data["group_name"], normalizeKeyGroup(group))
			}
			id := int64String(int64(data["id"].(float64)))
			updated := invokeKeyWithClaims(t, handler.UpdateAPIKey, admin, http.MethodPut, body, id)
			if updated.Code != http.StatusOK {
				t.Fatalf("existing group update rejected: status=%d body=%s", updated.Code, updated.Body.String())
			}
		})
	}
}

func TestAPIKeyCanDisableKeyWithRemovedPricingGroup(t *testing.T) {
	handler, repo := newChannelTestHandler(t)
	key := &storage.APIKeyRecord{Key: "sk-removed-group-synthetic", TenantID: "test", GroupName: "removed-group", Status: "active"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	w := invokeKeyWithClaims(t, handler.UpdateAPIKey, &AdminClaims{Username: "admin", Role: "admin"}, http.MethodPut,
		`{"status":"disabled"}`, int64String(key.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("status update should not reassign the group: status=%d body=%s", w.Code, w.Body.String())
	}
	updated, err := repo.GetAPIKey(key.ID)
	if err != nil || updated.Status != "disabled" || updated.GroupName != key.GroupName {
		t.Fatalf("unexpected persisted key: key=%v err=%v", updated, err)
	}
}
