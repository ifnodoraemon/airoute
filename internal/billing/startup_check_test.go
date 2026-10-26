package billing

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestResolvePriceSource(t *testing.T) {
	engine := NewEngine(nil) // hardcoded fallback only
	if got := engine.ResolvePriceSource("anything", "default"); got != PriceSourceDefault {
		t.Fatalf("unpriced model source = %q, want %q", got, PriceSourceDefault)
	}

	engine = &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:exact-model": {Model: "exact-model", GroupName: "default"},
			"vip:group-model":     {Model: "group-model", GroupName: "vip"},
			"vip:*":               {Model: "*", GroupName: "vip"},
		},
		wildcard:         []wildcardRule{{group: "default", prefix: "gpt-4", price: &storage.ModelPriceRecord{Model: "gpt-4*"}}},
		groupMultipliers: map[string]float64{"default": 1, "vip": 0.8},
		deflt:            &storage.ModelPriceRecord{Model: "*"},
	}
	cases := map[string]struct {
		model string
		group string
		want  string
	}{
		"exact":              {"exact-model", "default", PriceSourceExact},
		"group-specific":     {"group-model", "vip", PriceSourceExact},
		"prefix-wildcard":    {"gpt-4o-mini", "default", PriceSourceWildcard},
		"group-star-rule":    {"anything-else", "vip", PriceSourceWildcard},
		"unconfigured-model": {"never-configured", "default", PriceSourceDefault},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := engine.ResolvePriceSource(tc.model, tc.group); got != tc.want {
				t.Fatalf("source = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnpricedModelsInUse(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	user := &storage.UserRecord{Username: "pricing-check", PasswordHash: "synthetic", Balance: 10}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &storage.APIKeyRecord{Key: "sk-pricing-check", UserID: user.ID, TenantID: user.Username}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{Model: "priced-model", GroupName: "default", PromptPrice: 1, CompletionPrice: 4, Currency: "CNY"}); err != nil {
		t.Fatal(err)
	}

	records := []*storage.UsageLogRecord{
		{TraceID: "tr-priced", APIKey: key.Key, TenantID: user.Username, Model: "priced-model", Cost: 0.01, StatusCode: 200, BillingID: "bill-priced"},
		{TraceID: "tr-unpriced", APIKey: key.Key, TenantID: user.Username, Model: "mystery-model", Cost: 0.02, StatusCode: 200, BillingID: "bill-unpriced"},
	}
	if err := repo.BatchRecordUsageLogsContext(t.Context(), records); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo)
	unpriced, err := UnpricedModelsInUse(repo, engine, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(unpriced) != 1 || unpriced[0] != "mystery-model" {
		t.Fatalf("unpriced = %v, want [mystery-model]", unpriced)
	}
}
