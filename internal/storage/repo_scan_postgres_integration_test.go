//go:build integration

package storage

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPostgresReadProjections(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	checkRepositoryReadProjections(t, NewRepository(db))
}
