package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newCostTestHandler(t *testing.T) (*AdminHandler, *storage.DB) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewAdminHandler(storage.NewRepository(db), nil, nil), db
}

func invokeCost(t *testing.T, handler gin.HandlerFunc, method, target, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	handler(c)
	return w
}

func TestSupplierCostAPIAndMarginOverview(t *testing.T) {
	handler, db := newCostTestHandler(t)

	save := invokeCost(t, handler.SaveSupplierCost, http.MethodPost, "/api/v1/admin/supplier-costs",
		`{"channel":"ch-a","model":"m-1","input_cost":2,"output_cost":4}`, nil)
	if save.Code != http.StatusOK {
		t.Fatalf("save status=%d %s", save.Code, save.Body.String())
	}
	saved := decodeEnvelope(t, save)["data"].(map[string]interface{})
	if saved["input_cost"] != float64(2) || saved["output_cost"] != float64(4) || saved["status"] != "active" {
		t.Fatalf("saved = %+v", saved)
	}

	list := invokeCost(t, handler.ListSupplierCosts, http.MethodGet, "/api/v1/admin/supplier-costs", "", nil)
	items := decodeEnvelope(t, list)["data"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("supplier costs = %d", len(items))
	}

	// One priced request: ¥3 billed, ¥2 supplier cost.
	if _, err := db.Exec(`INSERT INTO usage_logs
		(api_key, tenant_id, org_id, model, channel, prompt_tokens, completion_tokens, total_tokens, cost, cost_units, cost_basis_units, status_code, created_at)
		VALUES ('k', 't', 0, 'm-1', 'ch-a', 1000000, 0, 1000000, 3, 3000000000, 2000000000, 200, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO usage_logs
		(api_key, tenant_id, org_id, model, channel, prompt_tokens, completion_tokens, total_tokens, cost, cost_units, status_code, created_at)
		VALUES ('k', 't', 0, 'm-2', 'ch-b', 10, 0, 10, 1, 1000000000, 200, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed unpriced usage: %v", err)
	}

	overview := invokeCost(t, handler.GetCostOverview, http.MethodGet, "/api/v1/admin/cost/overview", "", nil)
	data := decodeEnvelope(t, overview)["data"].(map[string]interface{})
	if data["revenue"] != float64(4) || data["cost"] != float64(2) || data["margin"] != float64(2) {
		t.Fatalf("overview = %+v", data)
	}
	if data["margin_percent"] != float64(50) {
		t.Fatalf("margin percent = %v", data["margin_percent"])
	}
	if data["known_cost_requests"] != float64(1) || data["unknown_cost_requests"] != float64(1) {
		t.Fatalf("cost coverage = %+v", data)
	}

	breakdown := invokeCost(t, handler.GetCostBreakdown, http.MethodGet, "/api/v1/admin/cost/breakdown?group=channel", "", nil)
	groups := decodeEnvelope(t, breakdown)["data"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	first := groups[0].(map[string]interface{})
	if first["key"] != "ch-a" || first["margin"] != float64(1) {
		t.Fatalf("first group = %+v", first)
	}

	badGroup := invokeCost(t, handler.GetCostBreakdown, http.MethodGet, "/api/v1/admin/cost/breakdown?group=tenant", "", nil)
	if badGroup.Code != http.StatusBadRequest {
		t.Fatalf("bad group status = %d", badGroup.Code)
	}

	id := int64(saved["id"].(float64))
	del := invokeCost(t, handler.DeleteSupplierCost, http.MethodDelete, "/api/v1/admin/supplier-costs", "", gin.Params{{Key: "id", Value: int64String(id)}})
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d", del.Code)
	}
}

func TestSupplierCostCSVImportExport(t *testing.T) {
	handler, _ := newCostTestHandler(t)
	body := "channel,model,input_cost,output_cost\nch-a,m-1,2,4\nch-b,m-2,1.5,3\n"

	dry := invokeCost(t, handler.ImportSupplierCosts, http.MethodPost,
		"/api/v1/admin/supplier-costs/import?dry_run=1", body, nil)
	if dry.Code != http.StatusOK {
		t.Fatalf("dry run status=%d %s", dry.Code, dry.Body.String())
	}
	dryData := decodeEnvelope(t, dry)["data"].(map[string]interface{})
	if dryData["valid"] != float64(2) || dryData["dry_run"] != true {
		t.Fatalf("dry run = %+v", dryData)
	}

	invalid := invokeCost(t, handler.ImportSupplierCosts, http.MethodPost,
		"/api/v1/admin/supplier-costs/import?dry_run=1", "channel,model\nch-a,m-1\n", nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d", invalid.Code)
	}

	imported := invokeCost(t, handler.ImportSupplierCosts, http.MethodPost,
		"/api/v1/admin/supplier-costs/import", body, nil)
	if imported.Code != http.StatusOK {
		t.Fatalf("import status=%d %s", imported.Code, imported.Body.String())
	}
	if decodeEnvelope(t, imported)["data"].(map[string]interface{})["imported"] != float64(2) {
		t.Fatalf("import response = %s", imported.Body.String())
	}

	exported := invokeCost(t, handler.ExportSupplierCosts, http.MethodGet, "/api/v1/admin/supplier-costs/export", "", nil)
	if exported.Code != http.StatusOK || !strings.Contains(exported.Body.String(), "channel,model") || !strings.Contains(exported.Body.String(), "ch-a") {
		t.Fatalf("export = %d %s", exported.Code, exported.Body.String())
	}
}

func TestCostTrendUsesDailyRollup(t *testing.T) {
	handler, db := newCostTestHandler(t)
	if _, err := db.Exec(`INSERT INTO usage_logs
		(api_key, tenant_id, org_id, model, channel, prompt_tokens, total_tokens, cost, cost_units, cost_basis_units, status_code, created_at)
		VALUES ('k', 't', 1, 'm-1', 'ch-a', 100, 100, 2, 2000000000, 1500000000, 200, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Format("2006-01-02")
	trend := invokeCost(t, handler.GetCostTrend, http.MethodGet,
		"/api/v1/admin/cost/trend?start="+day+"&end="+day, "", nil)
	if trend.Code != http.StatusOK {
		t.Fatalf("trend status=%d %s", trend.Code, trend.Body.String())
	}
	rows := decodeEnvelope(t, trend)["data"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("trend rows = %+v", rows)
	}
	row := rows[0].(map[string]interface{})
	if row["key"] != day || row["revenue"] != float64(2) || row["cost"] != float64(1.5) {
		t.Fatalf("trend row = %+v", row)
	}
}
