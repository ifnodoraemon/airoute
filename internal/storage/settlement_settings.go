package storage

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// envMilliseconds reads a positive millisecond override as a duration.
func envMilliseconds(name string, dflt time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return dflt
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return dflt
	}
	return time.Duration(value) * time.Millisecond
}
