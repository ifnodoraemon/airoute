package tests

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type closureApplication struct {
	t      *testing.T
	engine *gin.Engine
	repo   *storage.Repository
	admin  string
}

func newClosureApplication(t *testing.T) *closureApplication {
	t.Helper()
	previousConfig, previousRepo := config.GetGlobalConfig(), storage.GetGlobalRepository()
	previousEngine, previousLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := storage.NewRepository(db)
	config.SetGlobalConfig(config.DefaultConfig())
	billing.InitGlobalEngine(repo)
	logger := storage.InitAsyncLogger(repo, 64, 1, 10*time.Millisecond)
	t.Cleanup(func() {
		logger.Stop()
		storage.GlobalAsyncLogger = previousLogger
		billing.GlobalEngine = previousEngine
		storage.SetGlobalRepository(previousRepo)
		config.SetGlobalConfig(previousConfig)
		_ = db.Close()
	})
	if err := repo.EnsureDefaultAdmin("admin", "closure-admin-password"); err != nil {
		t.Fatal(err)
	}
	dispatcher := routerForClosure(repo)
	app := &closureApplication{t: t, repo: repo, engine: dispatcher}
	login := app.call("POST", "/api/v1/auth/login", "", `{"username":"admin","password":"closure-admin-password"}`, 200)
	app.admin = closureData(t, login)["token"].(string)
	return app
}

func routerForClosure(repo *storage.Repository) *gin.Engine {
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	return api.SetupRouter(dispatcher, controlplane.NewAdminHandler(repo, sync, dispatcher))
}

func (a *closureApplication) call(method, path, token, body string, status int) *httptest.ResponseRecorder {
	a.t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	a.engine.ServeHTTP(w, req)
	if w.Code != status {
		a.t.Fatalf("%s %s: got %d, expected %d: %s", method, path, w.Code, status, w.Body.String())
	}
	return w
}

func closureData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Data
}

func (a *closureApplication) createChannel(name, baseURL string, priority int) int64 {
	a.t.Helper()
	body := fmt.Sprintf(`{"name":%q,"base_url":%q,"type":"openai","models":["remote-chat"],"protocols":["openai_chat"],"priority":%d,"weight":10}`, name, baseURL, priority)
	w := a.call("POST", "/api/v1/admin/channels", a.admin, body, 200)
	return int64(closureData(a.t, w)["id"].(float64))
}
