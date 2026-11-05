package controlplane

import (
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"net/http"
	"strings"
	"testing"
)

func TestSupplierQuoteAPIRequiredRatesAndCurrency(t *testing.T) {
	h, _ := newCostTestHandler(t)
	for _, body := range []string{
		`{"channel":"c","model":"m","output_cost":1}`,
		`{"channel":"c","model":"m","input_cost":null,"output_cost":1}`,
		`{"channel":"c","model":"m","billing_mode":"request"}`,
		`{"channel":"c","model":"m","input_cost":1,"output_cost":1,"currency":"US$"}`,
		`{"channel":"c","model":"m","input_cost":1,"output_cost":1,"cache_read_cost":-1}`,
		`{"channel":"c","model":"m","input_cost":1,"output_cost":1,"billing_mode":"image"}`,
	} {
		r := invokeCost(t, h.SaveSupplierCost, http.MethodPost, "/", body, nil)
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid quote accepted: %s -> %d", body, r.Code)
		}
	}
	body := `{"channel":"c","model":"m","currency":"usd","input_cost":0,"output_cost":2.5,"cache_read_cost":0,"cache_write_cost":0.000000001}`
	r := invokeCost(t, h.SaveSupplierCost, http.MethodPost, "/", body, nil)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	data := decodeEnvelope(t, r)["data"].(map[string]interface{})
	if data["currency"] != "USD" || data["billing_mode"] != "token" || data["cache_read_cost"] != float64(0) || data["cache_write_cost"] != float64(0.000000001) {
		t.Fatalf("saved=%+v", data)
	}
	r = invokeCost(t, h.SaveSupplierCost, http.MethodPost, "/", `{"channel":"c","model":"m","currency":"EUR","billing_mode":"request","fixed_cost":0}`, nil)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	data = decodeEnvelope(t, r)["data"].(map[string]interface{})
	if data["fixed_cost"] != float64(0) || data["cache_read_cost"] != nil {
		t.Fatalf("request quote=%+v", data)
	}
}

func TestSupplierQuoteCSVRoundTripAllRates(t *testing.T) {
	h, _ := newCostTestHandler(t)
	body := "channel,model,input_cost,output_cost,status,currency,billing_mode,cache_read_cost,cache_write_cost,fixed_cost\nc,m,1.000000001,2,active,USD,token,0,,\nc,image,,,disabled,EUR,request,,,0.05\n"
	r := invokeCost(t, h.ImportSupplierCosts, http.MethodPost, "/", body, nil)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	exported := invokeCost(t, h.ExportSupplierCosts, http.MethodGet, "/", "", nil)
	records, errs, err := parseSupplierCSV(exported.Body.String())
	if err != nil || len(errs) != 0 || len(records) != 2 {
		t.Fatalf("round trip: %v %+v %s", err, errs, exported.Body.String())
	}
	for _, r := range records {
		if r.Model == "m" && (r.Currency != "USD" || r.InputCostUnitsPerMillion != 1000000001 || r.CacheReadCostUnitsPerMillion == nil || *r.CacheReadCostUnitsPerMillion != 0 || r.CacheWriteCostUnitsPerMillion != nil) {
			t.Fatalf("token quote=%+v", r)
		}
		if r.Model == "image" && (r.Currency != "EUR" || r.BillingMode != "request" || r.FixedCostUnits == nil || *r.FixedCostUnits != storage.ToUnits(0.05)) {
			t.Fatalf("request quote=%+v", r)
		}
	}
	for _, csv := range []string{"channel,model,input_cost,output_cost\nc,m,,1", "channel,model,typo\nc,m,1"} {
		_, errs, err := parseSupplierCSV(csv)
		if err == nil && len(errs) == 0 {
			t.Fatalf("invalid CSV accepted: %s", csv)
		}
	}
}

func TestChannelModelCatalogUsesActualNamesAndSharedCapabilities(t *testing.T) {
	rec := &storage.ChannelRecord{Name: "proxy", Type: model.ProviderCustom, APIKey: channelSecret, Models: []string{"public", "family/test", "*"}, ModelMapping: map[string]string{"public": "actual", "family/*": "upstream/*"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual": {Thinking: &model.ThinkingConfig{Profile: "reasoning-only"}}}}
	view := channelViewFrom(rec)
	if strings.Join(view.UpstreamModels, ",") != "actual,upstream/test" {
		t.Fatalf("upstream names=%v", view.UpstreamModels)
	}
	cap := view.ThinkingCapabilities["actual"]
	if !cap.Known || !cap.On || cap.Off || cap.Source != "configured" {
		t.Fatalf("capability=%+v", cap)
	}
}

func TestSupplierCSVRetainsNineDecimalsAtLargeRates(t *testing.T) {
	records, errs, err := parseSupplierCSV("channel,model,input_cost,output_cost\nc,m,999999999.123456789,1.23e-9\n")
	if err != nil || len(errs) != 0 || len(records) != 1 {
		t.Fatalf("parse=%v %+v", err, errs)
	}
	if records[0].InputCostUnitsPerMillion != 999999999123456789 || records[0].OutputCostUnitsPerMillion != 1 {
		t.Fatalf("precision lost: %+v", records[0])
	}
	_, errs, err = parseSupplierCSV("channel,model,input_cost,output_cost\nc,m,1000000000.000000001,1\n")
	if err == nil && len(errs) == 0 {
		t.Fatal("rounded-over-limit quote accepted")
	}
}
