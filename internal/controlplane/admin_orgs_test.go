package controlplane

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newOrgTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func invokeOrg(t *testing.T, handler gin.HandlerFunc, method, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, "/api/v1/admin/orgs", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	handler(c)
	return w
}

func orgData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	envelope := decodeEnvelope(t, w)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data object: %s", w.Body.String())
	}
	return data
}

func orgID(t *testing.T, data map[string]interface{}) string {
	t.Helper()
	id, ok := data["id"].(float64)
	if !ok || id <= 0 {
		t.Fatalf("missing id: %+v", data)
	}
	return int64String(int64(id))
}

func TestOrgHTTPCRUDAndTree(t *testing.T) {
	handler, _ := newOrgTestHandler(t)

	unit := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"unit","name":"宁夏移动","code":"nx"}`, nil))
	unitID := orgID(t, unit)
	if unit["level"] != "unit" || unit["path"] == "" {
		t.Fatalf("unit = %+v", unit)
	}

	dept := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"parent_id":`+unitID+`,"level":"department","name":"算力运营","code":"nx-compute"}`, nil))
	deptID := orgID(t, dept)
	project := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"parent_id":`+deptID+`,"level":"project","name":"创新业务","code":"nx-innov"}`, nil))
	projectID := orgID(t, project)

	got := orgData(t, invokeOrg(t, handler.GetOrg, http.MethodGet, "", gin.Params{{Key: "id", Value: projectID}}))
	if got["code"] != "nx-innov" {
		t.Fatalf("GetOrg = %+v", got)
	}

	list := invokeOrg(t, handler.ListOrgs, http.MethodGet, "", nil)
	envelope := decodeEnvelope(t, list)
	items, ok := envelope["data"].([]interface{})
	if !ok || len(items) < 4 { // default + unit + department + project
		t.Fatalf("list = %s", list.Body.String())
	}

	// Rename and archive.
	renamed := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"parent_id":`+deptID+`,"level":"project","name":"临时","code":"nx-temp"}`, nil))
	renamedID := orgID(t, renamed)
	updated := orgData(t, invokeOrg(t, handler.UpdateOrg, http.MethodPut,
		`{"name":"创新业务二部","status":"active"}`, gin.Params{{Key: "id", Value: renamedID}}))
	if updated["name"] != "创新业务二部" {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestOrgHTTPMoveAndMembers(t *testing.T) {
	handler, repo := newOrgTestHandler(t)

	unitA := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"unit","name":"A","code":"ua"}`, nil))
	unitAID := orgID(t, unitA)
	unitB := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"unit","name":"B","code":"ub"}`, nil))
	unitBID := orgID(t, unitB)
	dept := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"parent_id":`+unitAID+`,"level":"department","name":"D","code":"d1"}`, nil))
	deptID := orgID(t, dept)

	moved := orgData(t, invokeOrg(t, handler.MoveOrg, http.MethodPost,
		`{"parent_id":`+unitBID+`}`, gin.Params{{Key: "id", Value: deptID}}))
	if moved["parent_id"] != float64(mustInt(t, unitBID)) {
		t.Fatalf("moved = %+v", moved)
	}

	user := &storage.UserRecord{Username: "org-api-user", PasswordHash: "x"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	members := invokeOrg(t, handler.AddOrgMember, http.MethodPost,
		`{"user_id":`+int64String(user.ID)+`,"role":"admin"}`, gin.Params{{Key: "id", Value: deptID}})
	envelope := decodeEnvelope(t, members)
	items, ok := envelope["data"].([]interface{})
	if members.Code != http.StatusOK || !ok || len(items) != 1 {
		t.Fatalf("members = %d %s", members.Code, members.Body.String())
	}

	removed := invokeOrg(t, handler.RemoveOrgMember, http.MethodDelete, "",
		gin.Params{{Key: "id", Value: deptID}, {Key: "user_id", Value: int64String(user.ID)}})
	if removed.Code != http.StatusOK {
		t.Fatalf("remove = %d %s", removed.Code, removed.Body.String())
	}
}

func TestOrgHTTPValidation(t *testing.T) {
	handler, _ := newOrgTestHandler(t)

	// Department without a parent.
	bad := invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"department","name":"Bad","code":"bad"}`, nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", bad.Code, bad.Body.String())
	}

	// Unknown organization.
	missing := invokeOrg(t, handler.GetOrg, http.MethodGet, "", gin.Params{{Key: "id", Value: "99999"}})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", missing.Code, missing.Body.String())
	}

	// Duplicate code.
	unit := orgData(t, invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"unit","name":"U","code":"dup"}`, nil))
	_ = unit
	dup := invokeOrg(t, handler.CreateOrg, http.MethodPost,
		`{"level":"unit","name":"U2","code":"dup"}`, nil)
	if dup.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", dup.Code, dup.Body.String())
	}
}

func mustInt(t *testing.T, raw string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return value
}
