package controlplane

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newBudgetTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.CreateBudgets(db); err != nil {
		t.Fatalf("create budget schema: %v", err)
	}
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokeBudget(t *testing.T, handler gin.HandlerFunc, method, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, "/api/v1/admin/budgets", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	handler(c)
	return w
}

func TestBudgetHTTPCRUDEventsAndValidation(t *testing.T) {
	handler, repo := newBudgetTestHandler(t)
	org := &storage.OrgRecord{Level: storage.OrgLevelUnit, Name: "预算单位", Code: "budget-unit"}
	if err := repo.CreateOrg(org); err != nil {
		t.Fatal(err)
	}

	create := invokeBudget(t, handler.SaveBudget, http.MethodPost, fmt.Sprintf(
		`{"scope_type":"org","scope_id":%d,"period":"monthly","amount":1000,"action":"disable","warn_percent":80}`, org.ID), nil)
	data := budgetData(t, create)
	budgetID := int64(data["id"].(float64))
	if data["amount"] != float64(1000) || data["used"] != float64(0) || data["percent"] != float64(0) {
		t.Fatalf("create = %+v", data)
	}
	if data["action"] != "disable" || data["status"] != "active" {
		t.Fatalf("create defaults = %+v", data)
	}

	if err := repo.IncrementBudgetUsage(budgetID, storage.ToUnits(250)); err != nil {
		t.Fatal(err)
	}
	list := invokeBudget(t, handler.ListBudgets, http.MethodGet, "", nil)
	listEnvelope := decodeEnvelope(t, list)
	items, ok := listEnvelope["data"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list = %s", list.Body.String())
	}
	first := items[0].(map[string]interface{})
	if first["used"] != float64(250) || first["percent"] != float64(25) {
		t.Fatalf("usage not reflected: %+v", first)
	}

	update := invokeBudget(t, handler.UpdateBudget, http.MethodPut,
		`{"amount":2000,"action":"notify","warn_percent":90}`, gin.Params{{Key: "id", Value: int64String(budgetID)}})
	updated := budgetData(t, update)
	if updated["amount"] != float64(2000) || updated["action"] != "notify" || updated["warn_percent"] != float64(90) {
		t.Fatalf("update = %+v", updated)
	}
	if updated["used"] != float64(250) {
		t.Fatalf("update erased measured usage: %+v", updated)
	}

	if _, err := repo.RecordBudgetEvent(budgetID, 80, "2026-10", storage.ToUnits(800), storage.ToUnits(1000), "notify"); err != nil {
		t.Fatal(err)
	}
	events := invokeBudget(t, handler.ListBudgetEvents, http.MethodGet, "", gin.Params{{Key: "id", Value: int64String(budgetID)}})
	eventEnvelope := decodeEnvelope(t, events)
	if eventItems, ok := eventEnvelope["data"].([]interface{}); !ok || len(eventItems) != 1 {
		t.Fatalf("events = %s", events.Body.String())
	}

	// Validation: amount and scope.
	badAmount := invokeBudget(t, handler.SaveBudget, http.MethodPost, fmt.Sprintf(
		`{"scope_type":"org","scope_id":%d,"period":"monthly","amount":0}`, org.ID), nil)
	if badAmount.Code != http.StatusBadRequest {
		t.Fatalf("amount status = %d", badAmount.Code)
	}
	badScope := invokeBudget(t, handler.SaveBudget, http.MethodPost,
		`{"scope_type":"org","scope_id":99999,"period":"monthly","amount":10}`, nil)
	if badScope.Code != http.StatusBadRequest {
		t.Fatalf("scope status = %d %s", badScope.Code, badScope.Body.String())
	}
	badAction := invokeBudget(t, handler.SaveBudget, http.MethodPost, fmt.Sprintf(
		`{"scope_type":"org","scope_id":%d,"period":"monthly","amount":10,"action":"degrade"}`, org.ID), nil)
	if badAction.Code != http.StatusBadRequest {
		t.Fatalf("action status = %d", badAction.Code)
	}

	deleteResp := invokeBudget(t, handler.DeleteBudget, http.MethodDelete, "", gin.Params{{Key: "id", Value: int64String(budgetID)}})
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", deleteResp.Code, deleteResp.Body.String())
	}
	missing := invokeBudget(t, handler.DeleteBudget, http.MethodDelete, "", gin.Params{{Key: "id", Value: int64String(budgetID)}})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("delete missing = %d", missing.Code)
	}
}

func budgetData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	envelope := decodeEnvelope(t, w)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data: %s", w.Body.String())
	}
	return data
}
