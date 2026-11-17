//go:build integration

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresTimeSeriesAttentionSummary(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	checkTimeSeriesAttention(t, NewRepository(db))
}
