package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestSupplierCostAPI(t *testing.T) {
	handler, _ := newCostTestHandler(t)

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
