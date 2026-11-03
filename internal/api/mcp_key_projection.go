package api

import (
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// presentMCPLogKeys names each audited key the way an operator identifies it: the
// stored digest becomes the public ak_… identifier, and a digest that no longer
// resolves to a key row is masked instead of being returned raw.
//
// The audit columns intentionally persist the digest so the ledger joins keep
// working, so every read path has to project it for display. This mirrors
// controlplane.presentLogKeys on purpose but is a local copy: internal/api must
// not depend on internal/controlplane, and the MCP tool needs only the display
// projection (there is no reveal_key opt-in to honour here).
func (h *MCPHandler) presentMCPLogKeys(logs []*storage.UsageLogRecord) {
	if len(logs) == 0 {
		return
	}
	var publicIDs map[string]string
	if h.repo != nil {
		digests := make([]string, 0, len(logs))
		for _, rec := range logs {
			if rec != nil && rec.APIKey != "" && !storage.IsAPIKeyPublicID(rec.APIKey) {
				digests = append(digests, rec.APIKey)
			}
		}
		publicIDs, _ = h.repo.APIKeyPublicIDsByDigest(digests)
	}
	for _, rec := range logs {
		if rec == nil || rec.APIKey == "" || storage.IsAPIKeyPublicID(rec.APIKey) {
			continue
		}
		if publicID := publicIDs[rec.APIKey]; publicID != "" {
			rec.APIKey = publicID
			continue
		}
		// No repository (or the digest no longer resolves to a key row): mask it,
		// never return the raw digest — the point of the projection is that the
		// client only ever sees an identifier.
		rec.APIKey = storage.MaskAPIKey(rec.APIKey)
	}
}
