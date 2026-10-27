package controlplane

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// failingSnapshotSource simulates storage being unavailable during the first
// configuration load.
type failingSnapshotSource struct{}

func (failingSnapshotSource) ToModelChannels() ([]model.ChannelConfig, error) {
	return nil, errors.New("database unavailable")
}
func (failingSnapshotSource) ToModelAPIKeys() ([]model.APIKeyConfig, error) {
	return nil, errors.New("database unavailable")
}
func (failingSnapshotSource) HasRegisteredAPIKeys() (bool, error) {
	return false, errors.New("database unavailable")
}
func (failingSnapshotSource) GetModelFallbacks() (map[string]string, error) {
	return nil, errors.New("database unavailable")
}

// TestSynchronizerReadinessKeepsDataPlaneClosedOnFailure is the A03 regression.
//
// A failed first snapshot load used to leave the runtime with no key registered,
// which the auth middleware reads as "open dev mode" and admits every caller
// anonymously. The synchronizer must only announce readiness after a complete
// snapshot is installed, and a later success must recover the gateway.
func TestSynchronizerReadinessKeepsDataPlaneClosedOnFailure(t *testing.T) {
	previousConfig := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previousConfig)
	previousReady := middleware.DataPlaneReady()
	defer middleware.SetDataPlaneReady(previousReady)

	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)

	middleware.SetDataPlaneReady(false)
	sync := &Synchronizer{source: failingSnapshotSource{}, keys: globalKeyConfigSink{}}
	readyCalls := 0
	sync.SetOnReady(func() {
		readyCalls++
		middleware.SetDataPlaneReady(true)
	})

	if err := sync.ReloadFromDB(); err == nil {
		t.Fatal("expected the failed snapshot load to be reported")
	}
	if sync.Ready() {
		t.Fatal("synchronizer must not report ready after a failed load")
	}
	if middleware.DataPlaneReady() {
		t.Fatal("data plane must stay closed after a failed first load")
	}
	if readyCalls != 0 {
		t.Fatalf("readiness callback fired %d times on failure", readyCalls)
	}

	// Storage recovers: the next successful load must open the data plane.
	sync.source = repo
	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("recovery reload: %v", err)
	}
	if !sync.Ready() || !middleware.DataPlaneReady() {
		t.Fatal("expected readiness after a successful load")
	}
	if readyCalls != 1 {
		t.Fatalf("readiness callback fired %d times, want exactly 1", readyCalls)
	}

	// Further reloads must not fire it again.
	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	if readyCalls != 1 {
		t.Fatalf("readiness callback fired again (%d)", readyCalls)
	}
}

// TestSynchronizerSetOnReadyAfterReadyFiresImmediately covers late registration:
// a caller that wires the callback after the initial load must still learn the
// data plane is ready.
func TestSynchronizerSetOnReadyAfterReadyFiresImmediately(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)

	sync := &Synchronizer{source: repo, keys: globalKeyConfigSink{}}
	if err := sync.ReloadFromDB(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	fired := make(chan struct{}, 1)
	sync.SetOnReady(func() { fired <- struct{}{} })
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("SetOnReady after the first snapshot must invoke the callback")
	}
}

// TestUnreadyDataPlaneIsNotAnonymousEvenWithRegisteredKeys ties the synchronizer
// readiness to the data-plane middleware: the deployment has API keys in
// storage, but the first load failed, so the in-memory snapshot is empty. Such a
// gateway must answer 503 instead of falling into the open-dev-mode bypass.
func TestUnreadyDataPlaneIsNotAnonymousEvenWithRegisteredKeys(t *testing.T) {
	previousConfig := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previousConfig)
	previousReady := middleware.DataPlaneReady()
	defer middleware.SetDataPlaneReady(previousReady)
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	// The credential exists in durable storage...
	if err := repo.CreateAPIKey(&storage.APIKeyRecord{
		Key: "sk-registered-key", TenantID: "t", Status: "active", GroupName: "default",
	}); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// ...but the process could not load it, so its runtime key set is empty.
	emptyConfig := &config.Config{}
	emptyConfig.UpdateAPIKeys(nil, false)
	config.SetGlobalConfig(emptyConfig)

	sync := &Synchronizer{source: failingSnapshotSource{}, keys: globalKeyConfigSink{}}
	middleware.SetDataPlaneReady(false)
	sync.SetOnReady(func() { middleware.SetDataPlaneReady(true) })
	if err := sync.ReloadFromDB(); err == nil {
		t.Fatal("expected the snapshot load to fail")
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready data plane with registered keys = %d, want 503 (anonymous access must be impossible)", w.Code)
	}
}
