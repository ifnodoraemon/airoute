//go:build integration

package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresConsoleSelectedKeyState(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	require.NoError(t, err)
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{Username: "console-pg-" + itSuffix(), Role: "user", Status: "active", Balance: 12.34}
	require.NoError(t, repo.CreateUser(user))
	defer db.Exec(`DELETE FROM users WHERE id=?`, user.ID)
	key := &APIKeyRecord{Key: "sk-console-pg-" + itSuffix(), TenantID: user.Username, UserID: user.ID, Status: "active", AllowedModels: []string{"chat-a"}, RPM: 60, TPM: 1000, Budget: 5}
	require.NoError(t, repo.CreateAPIKey(key))
	defer db.Exec(`DELETE FROM api_keys WHERE id=?`, key.ID)

	selected, err := repo.GetAPIKeyContext(t.Context(), key.ID)
	require.NoError(t, err)
	require.Equal(t, key.Key, selected.Key)
	require.Equal(t, key.PublicID, selected.PublicID)
	require.Equal(t, user.ID, selected.UserID)
	require.Equal(t, user.Status, selected.UserStatus)
	require.Equal(t, user.Role, selected.UserRole)
	require.InDelta(t, user.Balance, selected.UserBalance, 1e-9)
	require.Equal(t, key.AllowedModels, selected.ToConfig().AllowedModels)

	// A different replica can revoke a key without updating this process's
	// snapshot. Selection must read that durable state, including deleted rows.
	key.Status, key.RPM = "disabled", 3
	require.NoError(t, repo.UpdateAPIKey(key))
	selected, err = repo.GetAPIKeyContext(t.Context(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "disabled", selected.Status)
	require.Equal(t, 3, selected.RPM)
	require.NoError(t, repo.DeleteAPIKey(key.ID))
	selected, err = repo.GetAPIKeyContext(t.Context(), key.ID)
	require.NoError(t, err)
	require.Equal(t, "deleted", selected.Status)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = repo.GetAPIKeyContext(ctx, key.ID)
	require.True(t, errors.Is(err, context.Canceled), "selected-key lookup must honor request cancellation: %v", err)
}
