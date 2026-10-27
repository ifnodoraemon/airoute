package controlplane

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// jsonFieldNames returns the set of JSON names a struct accepts or emits, with
// embedded structs flattened the way encoding/json flattens them.
//
// This is the seed of a machine-checkable description of the admin API surface.
// The console reads these names as plain property accesses, so a rename on either
// side compiles everywhere and fails only in a browser.
func jsonFieldNames(t reflect.Type) map[string]bool {
	out := map[string]bool{}

	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous {
				walk(f.Type)
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name == "" {
				name = f.Name
			}
			out[name] = true
		}
	}
	walk(t)

	return out
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sameNameSet(got map[string]bool, want []string) (missing, extra []string) {
	wantSet := make(map[string]bool, len(want))
	for _, name := range want {
		wantSet[name] = true
		if !got[name] {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if !wantSet[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// TestAdminRequestShapesAreStable pins the exact JSON names the console may send.
//
// It is an inventory rather than a sample: adding a field is fine, but renaming
// or removing one has to be a deliberate edit here as well, which is what turns
// an accidental break into a failing build.
func TestAdminRequestShapesAreStable(t *testing.T) {
	cases := []struct {
		name string
		got  map[string]bool
		want []string
	}{
		{
			name: "UpdateModelRouteRequest",
			got:  jsonFieldNames(reflect.TypeOf(UpdateModelRouteRequest{})),
			want: []string{
				"model", "group_name", "fallback_model", "provider_updates",
				"prompt_price", "completion_price", "cache_read_price", "fixed_price",
				"currency", "off_peak_enabled", "off_peak_mode", "off_peak_slots",
				"weekend_all_day", "off_peak_start", "off_peak_end", "off_peak_discount",
			},
		},
		{
			name: "ModelProviderUpdate",
			got:  jsonFieldNames(reflect.TypeOf(ModelProviderUpdate{})),
			want: []string{"channel_id", "priority", "weight", "mapped_model"},
		},
		{
			name: "SavePricingRateRequest",
			got:  jsonFieldNames(reflect.TypeOf(SavePricingRateRequest{})),
			want: []string{
				"model", "group_name", "prompt_price", "completion_price",
				"cache_read_price", "fixed_price", "currency", "off_peak_enabled",
				"off_peak_mode", "off_peak_slots", "weekend_all_day",
				"off_peak_start", "off_peak_end", "off_peak_discount",
			},
		},
		{
			name: "BatchStatusRequest",
			got:  jsonFieldNames(reflect.TypeOf(BatchStatusRequest{})),
			want: []string{"ids", "status"},
		},
		{
			name: "BatchIDsRequest",
			got:  jsonFieldNames(reflect.TypeOf(BatchIDsRequest{})),
			want: []string{"ids"},
		},
		{
			name: "ChannelRequest",
			got:  jsonFieldNames(reflect.TypeOf(ChannelRequest{})),
			want: []string{
				"name", "type", "base_url", "api_key", "models", "model_mapping",
				"protocols", "priority", "weight", "timeout_seconds", "status",
			},
		},
		{
			name: "CreateAPIKeyRequest",
			got:  jsonFieldNames(reflect.TypeOf(CreateAPIKeyRequest{})),
			want: []string{
				"key", "tenant_id", "user_id", "allowed_models", "rpm", "tpm",
				"budget", "group_name", "status", "format_validation",
			},
		},
		{
			// This one used to be an anonymous struct declared inside the handler,
			// so its shape was outside the pinned contract entirely.
			name: "UpdateAPIKeyRequest",
			got:  jsonFieldNames(reflect.TypeOf(UpdateAPIKeyRequest{})),
			want: []string{
				"tenant_id", "allowed_models", "rpm", "tpm", "budget", "status",
				"format_validation", "group_name",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing, extra := sameNameSet(tc.got, tc.want)
			if len(missing) > 0 {
				t.Errorf("fields the console sends that this type no longer accepts: %v", missing)
			}
			if len(extra) > 0 {
				t.Errorf("fields this type accepts that are not in the pinned contract: %v", extra)
			}
		})
	}
}

// TestSavePricingRateRequestAcceptsExactlyTheWritableColumns is the compatibility
// check for the pricing save endpoint.
//
// The request used to be storage.ModelPriceRecord itself, so the accepted set was
// whatever the row happened to expose. Now that they are separate, the request
// must keep accepting every column the repository actually writes, or a console
// save would silently stop applying part of its payload.
func TestSavePricingRateRequestAcceptsExactlyTheWritableColumns(t *testing.T) {
	request := jsonFieldNames(reflect.TypeOf(SavePricingRateRequest{}))
	record := jsonFieldNames(reflect.TypeOf(storage.ModelPriceRecord{}))

	// Generated by the repository; a client must not be able to choose them.
	outputOnly := map[string]bool{"id": true, "created_at": true, "updated_at": true}

	for _, name := range sortedNames(record) {
		if outputOnly[name] {
			continue
		}
		if !request[name] {
			t.Errorf("the request no longer accepts %q, which the stored record exposes and SaveModelPrice writes", name)
		}
	}

	for _, name := range sortedNames(request) {
		if !record[name] {
			t.Errorf("the request accepts %q, which the stored record does not have", name)
		}
		if outputOnly[name] {
			t.Errorf("the request accepts the output-only column %q", name)
		}
	}
}

// TestSavePricingRateIgnoresClientSuppliedRowMetadata pins the behaviour behind
// dropping those three columns: a console that echoes the whole row back must not
// be able to choose its identifier or timestamps.
func TestSavePricingRateIgnoresClientSuppliedRowMetadata(t *testing.T) {
	handler, repo := newPricingTestHandler(t)

	body := `{"id":9999,"created_at":"2000-01-01T00:00:00Z","updated_at":"2000-01-01T00:00:00Z",
	          "model":"gpt-x","group_name":"default","prompt_price":3}`

	w := invokePricing(t, handler.SavePricingRate, http.MethodPost, body, "/api/v1/admin/pricing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	stored, err := repo.GetModelPriceExact("gpt-x", "default")
	if err != nil {
		t.Fatalf("price was not persisted: %v", err)
	}
	if stored.ID == 9999 {
		t.Error("the client chose the row id")
	}
	if stored.CreatedAt.Year() == 2000 || stored.UpdatedAt.Year() == 2000 {
		t.Errorf("the client chose the row timestamps: created=%v updated=%v", stored.CreatedAt, stored.UpdatedAt)
	}
	if stored.PromptPrice != 3 {
		t.Errorf("prompt price = %v, want 3", stored.PromptPrice)
	}
}

// TestChannelRequestAcceptsExactlyTheWritableColumns is the compatibility check
// for the channel create and update endpoints.
func TestChannelRequestAcceptsExactlyTheWritableColumns(t *testing.T) {
	request := jsonFieldNames(reflect.TypeOf(ChannelRequest{}))
	record := jsonFieldNames(reflect.TypeOf(storage.ChannelRecord{}))

	// Generated by the repository, or decorated at runtime by the dispatcher.
	// A client must not be able to choose any of them.
	outputOnly := map[string]bool{
		"id":             true,
		"breaker_status": true,
		"created_at":     true,
		"updated_at":     true,
	}

	for _, name := range sortedNames(record) {
		if outputOnly[name] {
			continue
		}
		if !request[name] {
			t.Errorf("the request no longer accepts %q, which the stored record exposes and the channel writes persist", name)
		}
	}

	for _, name := range sortedNames(request) {
		if !record[name] {
			t.Errorf("the request accepts %q, which the stored record does not have", name)
		}
		if outputOnly[name] {
			t.Errorf("the request accepts the output-only column %q", name)
		}
	}
}

// TestCreateChannelIgnoresClientSuppliedRowMetadata pins the behaviour behind
// dropping those columns: a console that echoes a whole channel row back must not
// be able to choose its identifier, its circuit-breaker state or its timestamps.
func TestCreateChannelIgnoresClientSuppliedRowMetadata(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	body := `{"id":4242,"breaker_status":"OPEN","created_at":"2000-01-01T00:00:00Z",
	          "name":"chan-a","type":"openai","base_url":"http://a.invalid","api_key":"k","status":"active"}`

	w := invokeChannel(t, handler.CreateChannel, http.MethodPost, body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	list, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("stored channels = %d, want 1", len(list))
	}
	stored := list[0]
	if stored.ID == 4242 {
		t.Error("the client chose the row id")
	}
	if stored.BreakerStatus == "OPEN" {
		t.Error("the client set the circuit-breaker state")
	}
	if stored.CreatedAt.Year() == 2000 {
		t.Errorf("the client chose the creation timestamp: %v", stored.CreatedAt)
	}
	if stored.Name != "chan-a" || stored.APIKey != "k" {
		t.Errorf("the legitimate fields were not persisted: %+v", stored)
	}
}

// TestCreateAPIKeyRequestAcceptsExactlyTheWritableColumns is the compatibility
// check for the API key creation endpoint.
func TestCreateAPIKeyRequestAcceptsExactlyTheWritableColumns(t *testing.T) {
	request := jsonFieldNames(reflect.TypeOf(CreateAPIKeyRequest{}))
	record := jsonFieldNames(reflect.TypeOf(storage.APIKeyRecord{}))

	// Server-owned: the repository assigns the row metadata, the handler zeroes
	// the usage counters, and CreateAPIKey recomputes the integer ledger mirrors
	// from the float columns rather than trusting the body.
	serverOwned := map[string]bool{
		"id":              true,
		"used_tokens":     true,
		"used_cost":       true,
		"budget_units":    true,
		"used_cost_units": true,
		"created_at":      true,
		"updated_at":      true,
		// Fingerprint is derived from Key when the row is read or created; it is
		// never accepted from (or needed in) the creation body.
		"key_fingerprint": true,
		// Organization attribution is derived server-side (the owner's org) when
		// CreateAPIKey stores the row; accepting it in the body would let a tenant
		// label its traffic as another organization.
		"org_id": true,
	}

	for _, name := range sortedNames(record) {
		if serverOwned[name] {
			continue
		}
		if !request[name] {
			t.Errorf("the request no longer accepts %q, which the stored record exposes and CreateAPIKey writes", name)
		}
	}

	for _, name := range sortedNames(request) {
		if !record[name] {
			t.Errorf("the request accepts %q, which the stored record does not have", name)
		}
		if serverOwned[name] {
			t.Errorf("the request accepts the server-owned column %q", name)
		}
	}
}

// TestCreateAPIKeyIgnoresClientSuppliedAccounting pins the security-relevant half
// of dropping those columns: a client must not be able to seed a key with a
// fabricated spend, because a negative used_cost would raise the effective budget.
func TestCreateAPIKeyIgnoresClientSuppliedAccounting(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	body := `{"key":"sk-airoute-0123456789abcdef","tenant_id":"t1","budget":10,
	          "id":777,"used_cost":-9999,"used_tokens":5000,
	          "used_cost_units":-9999000000000,"budget_units":999999999999}`

	w := invokeChannel(t, handler.CreateAPIKey, http.MethodPost, body, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	keys, err := repo.ListAPIKeys()
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("stored keys = %d, want 1", len(keys))
	}
	got := keys[0]

	if got.ID == 777 {
		t.Error("the client chose the row id")
	}
	if got.UsedCost != 0 {
		t.Errorf("client-supplied used_cost survived: %v", got.UsedCost)
	}
	if got.UsedTokens != 0 {
		t.Errorf("client-supplied used_tokens survived: %d", got.UsedTokens)
	}
	if got.UsedCostUnits != 0 {
		t.Errorf("client-supplied used_cost_units survived: %d", got.UsedCostUnits)
	}
	// The mirrors must be derived from the float columns, not taken from the body.
	if got.BudgetUnits != storage.ToUnits(10) {
		t.Errorf("budget_units = %d, want %d (derived from budget=10)", got.BudgetUnits, storage.ToUnits(10))
	}
	if got.Budget != 10 {
		t.Errorf("budget = %v, want 10", got.Budget)
	}
}

// TestAdminResponseShapesAreStable pins the JSON names the console reads back.
//
// These responses are the storage records themselves, so the stored row IS the
// public contract: renaming a column compiles on both sides and fails only in a
// browser. Pinning the names here makes that coupling visible and turns an
// accidental rename into a failing build. Replacing these records with explicit
// response types is the rest of the interface-contract work.
func TestAdminResponseShapesAreStable(t *testing.T) {
	cases := []struct {
		name string
		got  map[string]bool
		want []string
	}{
		{
			name: "storage.ChannelRecord",
			got:  jsonFieldNames(reflect.TypeOf(storage.ChannelRecord{})),
			want: []string{
				"id", "name", "type", "base_url", "api_key", "models", "model_mapping",
				"protocols", "priority", "weight", "timeout_seconds", "status",
				"breaker_status", "created_at", "updated_at",
			},
		},
		{
			name: "storage.ModelPriceRecord",
			got:  jsonFieldNames(reflect.TypeOf(storage.ModelPriceRecord{})),
			want: []string{
				"id", "model", "group_name", "prompt_price", "completion_price",
				"cache_read_price", "fixed_price", "currency", "off_peak_enabled",
				"off_peak_mode", "off_peak_slots", "weekend_all_day",
				"off_peak_start", "off_peak_end", "off_peak_discount",
				"created_at", "updated_at",
			},
		},
		{
			name: "storage.APIKeyRecord",
			got:  jsonFieldNames(reflect.TypeOf(storage.APIKeyRecord{})),
			want: []string{
				"id", "key", "tenant_id", "allowed_models", "rpm", "tpm", "budget",
				"used_tokens", "used_cost", "budget_units", "used_cost_units",
				"group_name", "user_id", "org_id", "status", "format_validation",
				"created_at", "updated_at", "key_fingerprint",
			},
		},
		{
			// The route editor is built from this aggregate, so its field names are
			// as much of a contract as the request it sends back.
			name: "router.ModelRouteDetail",
			got:  jsonFieldNames(reflect.TypeOf(router.ModelRouteDetail{})),
			want: []string{
				"model", "modality", "providers", "primary_providers_count",
				"fallback_providers_count", "total_primary_weight", "fallback_model",
				"has_cross_protocol", "has_fallback_tier",
			},
		},
		{
			name: "router.ModelProviderInfo",
			got:  jsonFieldNames(reflect.TypeOf(router.ModelProviderInfo{})),
			want: []string{
				"channel_id", "channel_name", "channel_type", "base_url", "priority",
				"weight", "weight_percent", "mapped_model", "protocols",
				"breaker_status", "status", "conversion_needed", "conversion_type",
				"shared_model_count",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing, extra := sameNameSet(tc.got, tc.want)
			if len(missing) > 0 {
				t.Errorf("fields the console reads that this type no longer emits: %v", missing)
			}
			if len(extra) > 0 {
				t.Errorf("fields this type emits that are not in the pinned contract: %v", extra)
			}
		})
	}
}

// TestEnrichedPriceFlattensTheStoredRecord pins the shape of a pricing list row: the stored columns are inlined alongside the two live off-peak fields, which is
// what lets the console render a rate and its current discount from one object.
func TestEnrichedPriceFlattensTheStoredRecord(t *testing.T) {
	enriched := jsonFieldNames(reflect.TypeOf(EnrichedPrice{}))
	record := jsonFieldNames(reflect.TypeOf(storage.ModelPriceRecord{}))

	for _, name := range sortedNames(record) {
		if !enriched[name] {
			t.Errorf("the enriched row dropped the stored field %q; the pricing list renders it", name)
		}
	}
	// These names are the console's contract, not the gateway's preference.
	// PricingManager reads is_currently_off_peak / current_discount and the two
	// effective prices; when the gateway sent is_current_off_peak /
	// effective_discount instead, every value arrived undefined and the "live
	// effective rate" column showed the standard price for every model. Renaming
	// either side without the other reintroduces that silently.
	for _, name := range []string{
		"is_currently_off_peak", "current_discount",
		"effective_prompt_price", "effective_completion_price",
	} {
		if !enriched[name] {
			t.Errorf("the enriched row lost the enrichment field %q, which the pricing list reads", name)
		}
	}
}
