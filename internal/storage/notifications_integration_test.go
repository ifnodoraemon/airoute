//go:build integration

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresNotifications(t *testing.T) {
	db, err := OpenDB(partitionScratchDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	notificationStorageSuite(t, db)
}
