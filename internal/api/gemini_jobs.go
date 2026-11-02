package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// StartNativeJobReconciler finishes metering accepted asynchronous work even
// when a client never polls. Durable, scoped leases and billing IDs coordinate
// clients and both replicas; a crashed worker can be retried after its lease.
func StartNativeJobReconciler(ctx context.Context, dispatcher *router.Dispatcher) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcileNativeJobs(ctx, NewHandler(dispatcher))
			}
		}
	}()
}

func reconcileNativeJobs(ctx context.Context, h *Handler) {
	repo := storage.GetGlobalRepository()
	if repo == nil {
		return
	}
	jobs, err := repo.PendingNativeJobs(ctx)
	if err != nil {
		telemetry.ScopedLogger("system").Warn("failed to read pending native jobs", "error", err.Error())
		return
	}
	queue := make(chan storage.NativeState, len(jobs))
	for _, job := range jobs {
		queue <- job
	}
	close(queue)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range queue {
				if ctx.Err() != nil {
					return
				}
				_ = repo.TouchNativeJob(ctx, job)
				pollNativeJob(ctx, h, job)
			}
		}()
	}
	wg.Wait()
	_ = repo.PruneNativeStates(ctx)
}

func pollNativeJob(ctx context.Context, h *Handler, state storage.NativeState) {
	var key model.APIKeyConfig
	if json.Unmarshal(state.Billing, &key) != nil || key.Key == "" {
		return
	}
	path := "/v1beta/" + state.ID
	if state.Namespace == "gemini-interaction" {
		path = "/v1beta/interactions/" + state.ID
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return
	}
	c, _ := gin.CreateTestContext(&nativeJobWriter{header: make(http.Header)})
	c.Request = request
	c.Set(middleware.ContextKeyAPIKeyConfig, &key)
	c.Set("native_job_owner", state.Owner)
	r := &googleResource{namespace: state.Namespace, id: state.ID, model: state.Model, state: &state, path: path}
	if state.Namespace == "gemini-interaction" {
		r.collection = "interactions"
		h.handleGeminiInteraction(c, r)
	} else {
		r.collection = "batches"
		h.forwardGoogleResource(c, r)
	}
}

type nativeJobWriter struct{ header http.Header }

func (w *nativeJobWriter) Header() http.Header            { return w.header }
func (w *nativeJobWriter) WriteHeader(int)                {}
func (w *nativeJobWriter) Write(data []byte) (int, error) { return io.Discard.Write(data) }
