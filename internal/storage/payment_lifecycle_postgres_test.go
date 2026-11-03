//go:build integration

package storage

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPostgresPaymentLifecycle(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	require.NoError(t, err)
	defer db.Close()
	paymentStorageSuite(t, db)
}
