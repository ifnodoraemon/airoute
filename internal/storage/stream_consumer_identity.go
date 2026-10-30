package storage

import (
	"fmt"
	"os"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// streamConsumerID returns the identity this process registers as in the Redis
// Streams consumer group.
//
// Redis creates a consumer entry for every distinct name it sees and never removes
// it, so the identity choice decides whether the group accumulates one entry per
// process start or one per replica. NODE_ID (falling back to the container
// hostname) is the same identity the live-stats snapshots use, and the shipped
// Compose file pins it per gateway replica, which makes the name stable across
// restarts and redeploys.
//
// When NODE_ID is not set the host-pid fallback is kept: two ad-hoc processes on
// one host must not share a consumer name, because they would read each other's
// pending entries. That fallback grows one entry per start, which is why the read
// loop also sweeps idle consumers (PruneIdleConsumers).
func streamConsumerID() string {
	if id := strings.TrimSpace(os.Getenv("NODE_ID")); id != "" {
		return id
	}
	return fmt.Sprintf("%s-%d", telemetry.LiveNodeID(), os.Getpid())
}
