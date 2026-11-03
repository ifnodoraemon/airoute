//go:build integration

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresChannelRenameSupplierCosts(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	require.NoError(t, err)
	defer db.Close()
	channelRenameSupplierSuite(t, db)
}
