package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// AdminHandler handles REST endpoints for the Control Plane.
type AdminHandler struct {
	repo       *storage.Repository
	sync       *Synchronizer
	dispatcher *router.Dispatcher
	prober     *DownstreamProber
	storage    storage.ArtifactStorage

	// channels owns the channel business rules. Handlers delegate to it instead
	// of interleaving validation, persistence and hot sync themselves.
	channels *ChannelService

	// routes owns the model-route save rules.
	routes *RouteService

	// pricing owns the model-pricing rules.
	pricing *PricingService

	// keyAccess owns the API key ownership rule behind the single-key endpoints.
	keyAccess *KeyAccessRules

	// keyCreation owns how a key request becomes a storable row.
	keyCreation *KeyCreationRules
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(repo *storage.Repository, sync *Synchronizer, dispatcher *router.Dispatcher) *AdminHandler {
	ls, _ := storage.NewLocalStorage("data/storage")

	// The interface conversions below are guarded on purpose. A nil
	// *router.Dispatcher stored in a BreakerStatusReader is a non-nil interface,
	// so the service's own nil check would pass and the first call would panic:
	// GetBreakerStatus dereferences its receiver.
	var store ChannelStore
	if repo != nil {
		store = repo
	}
	// RouteStore needs a different subset of the repository, so it gets its own
	// guarded conversion rather than reusing the ChannelStore value.
	var routeStore RouteStore
	if repo != nil {
		routeStore = repo
	}
	// Likewise for the pricing subset.
	var pricingStore PricingStore
	if repo != nil {
		pricingStore = repo
	}
	// And the account lookup the key ownership rule needs.
	var keyOwners KeyOwnerLookup
	if repo != nil {
		keyOwners = repo
	}
	var reloader ChannelReloader
	if sync != nil {
		reloader = sync
	}
	var routeSync RouteSync
	if sync != nil {
		routeSync = sync
	}
	var pricingSync PricingSync
	if sync != nil {
		pricingSync = sync
	}
	var breakers BreakerStatusReader
	if dispatcher != nil {
		breakers = dispatcher
	}

	return &AdminHandler{
		repo:        repo,
		sync:        sync,
		dispatcher:  dispatcher,
		prober:      NewDownstreamProber(nil),
		storage:     ls,
		channels:    NewChannelService(store, reloader, breakers),
		routes:      NewRouteService(routeStore, routeSync, globalPriceReloader{}),
		pricing:     NewPricingService(pricingStore, pricingSync, globalPriceReloader{}, globalOffPeakEvaluator{}),
		keyAccess:   NewKeyAccessRules(keyOwners),
		keyCreation: NewKeyCreationRules(keyOwners),
	}
}

// SetArtifactStorage configures the pluggable artifact storage driver (Local or RustFS/S3).
func (h *AdminHandler) SetArtifactStorage(s storage.ArtifactStorage) {
	if s != nil {
		h.storage = s
	}
}

func (h *AdminHandler) getStorage() storage.ArtifactStorage {
	if h.storage != nil {
		return h.storage
	}
	ls, _ := storage.NewLocalStorage("data/storage")
	h.storage = ls
	return ls
}

// syncDataPlane synchronizes updated channels, API keys, and routing rules into memory and cluster replicas.
func (h *AdminHandler) syncDataPlane(events ...string) {
	if h.sync != nil {
		event := "data_plane_updated"
		if len(events) > 0 && events[0] != "" {
			event = events[0]
		}
		_ = h.sync.ReloadAndBroadcast(cContext(), event)
	}
}

func cContext() context.Context {
	return context.Background()
}

// GetRepo returns the underlying storage repository.
func (h *AdminHandler) GetRepo() *storage.Repository {
	if h == nil {
		return nil
	}
	return h.repo
}

// ResolvePublicBaseURL dynamically resolves the public base URL of the gateway
// prioritizing configuration/env (PUBLIC_URL, GATEWAY_PUBLIC_URL) or HTTP proxy headers.
func ResolvePublicBaseURL(c *gin.Context) string {
	cfg := config.GetGlobalConfig()
	if cfg != nil {
		if pub := cfg.GetPublicURL(); pub != "" {
			return pub
		}
	}

	if c != nil && c.Request != nil {
		proto := c.GetHeader("X-Forwarded-Proto")
		if proto == "" {
			if c.Request.TLS != nil {
				proto = "https"
			} else {
				proto = "http"
			}
		}

		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}

		if host != "" {
			return fmt.Sprintf("%s://%s", proto, host)
		}
	}

	if pub := os.Getenv("PUBLIC_URL"); pub != "" {
		return pub
	}
	if pub := os.Getenv("GATEWAY_PUBLIC_URL"); pub != "" {
		return pub
	}

	host := os.Getenv("GATEWAY_HOST")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	port := 8080
	if cfg != nil && cfg.Server.Port > 0 {
		port = cfg.Server.Port
	} else if p := os.Getenv("GATEWAY_INGRESS_PORT"); p != "" {
		if pi, err := strconv.Atoi(p); err == nil && pi > 0 {
			port = pi
		}
	} else if p := os.Getenv("GATEWAY_PORT"); p != "" {
		if pi, err := strconv.Atoi(p); err == nil && pi > 0 {
			port = pi
		}
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// BatchIDsRequest represents a request body containing a list of entity IDs.
type BatchIDsRequest struct {
	IDs []int64 `json:"ids"`
}

// BatchStatusRequest represents a request body for toggling the status of multiple entities.
type BatchStatusRequest struct {
	IDs    []int64 `json:"ids"`
	Status string  `json:"status"` // "active" or "disabled"
}

// bindBatchIDs binds a JSON body containing a non-empty list of IDs.
func (h *AdminHandler) bindBatchIDs(c *gin.Context, emptyErrMsg string) ([]int64, bool) {
	var req BatchIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		if emptyErrMsg == "" {
			emptyErrMsg = "请提供有效的 ID 列表"
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": emptyErrMsg})
		return nil, false
	}
	return req.IDs, true
}

// bindBatchStatus binds a JSON body containing IDs and a status ("active" or "disabled").
func (h *AdminHandler) bindBatchStatus(c *gin.Context) (*BatchStatusRequest, bool) {
	var req BatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求参数"})
		return nil, false
	}
	if req.Status != "active" && req.Status != "disabled" {
		req.Status = "active"
	}
	return &req, true
}
