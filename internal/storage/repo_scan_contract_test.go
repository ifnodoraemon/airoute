package storage

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

func TestRepositoryReadProjections(t *testing.T) {
	repo := newUserTestRepo(t)
	channels, err := repo.ListChannels()
	require.NoError(t, err)
	require.Nil(t, channels)
	keys, err := repo.ListAPIKeys()
	require.NoError(t, err)
	require.NotNil(t, keys)
	require.Empty(t, keys)
	checkRepositoryReadProjections(t, repo)
}

func checkRepositoryReadProjections(t *testing.T, repo *Repository) {
	t.Helper()
	suffix := fmt.Sprint(time.Now().UnixNano())
	user := &UserRecord{Username: "scan-" + suffix, Email: suffix + "@example.test", PasswordHash: "hash-kept-private", Balance: 9, GroupName: "vip"}
	require.NoError(t, repo.CreateUser(user))
	_, err := repo.db.Exec(`UPDATE users SET balance = 99, balance_units = ? WHERE id = ?`, int64(1234567891), user.ID)
	require.NoError(t, err)
	checkUserReadProjections(t, repo, user)
	checkKeyReadProjections(t, repo, user)
	checkChannelReadProjections(t, repo, suffix)
	checkPriceReadProjections(t, repo, suffix)
}

func checkUserReadProjections(t *testing.T, repo *Repository, user *UserRecord) {
	t.Helper()
	byID, err := repo.GetUserByID(user.ID)
	require.NoError(t, err)
	byName, err := repo.GetUserByUsername(user.Username)
	require.NoError(t, err)
	byEmail, err := repo.GetUserByEmail(user.Email)
	require.NoError(t, err)
	require.Equal(t, byID, byName)
	require.Equal(t, byID, byEmail)
	require.Equal(t, 1.234567891, byID.Balance)
	require.Equal(t, "hash-kept-private", byID.PasswordHash)
	users, err := repo.ListUsers()
	require.NoError(t, err)
	for _, listed := range users {
		if listed.ID != user.ID {
			continue
		}
		require.Empty(t, listed.PasswordHash)
		copy := *byID
		copy.PasswordHash = ""
		require.Equal(t, &copy, listed)
		return
	}
	t.Fatal("created user missing from list")
}

func checkKeyReadProjections(t *testing.T, repo *Repository, user *UserRecord) {
	t.Helper()
	key := &APIKeyRecord{Key: "credential-" + user.Username, TenantID: user.Username, UserID: user.ID, AllowedModels: []string{"first", "second"}, RPM: 17, TPM: 203, Budget: 4, FormatValidation: "strict"}
	require.NoError(t, repo.CreateAPIKey(key))
	_, err := repo.db.Exec(`UPDATE api_keys SET budget = 99, used_cost = 88, budget_units = ?, used_cost_units = ?, admin_disabled = TRUE WHERE id = ?`, int64(4000000001), int64(1000000003), key.ID)
	require.NoError(t, err)
	byID, err := repo.GetAPIKey(key.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, byID.AllowedModels)
	require.Equal(t, 4.000000001, byID.Budget)
	require.Equal(t, 1.000000003, byID.UsedCost)
	require.True(t, byID.AdminDisabled)
	require.Equal(t, "strict", byID.FormatValidation)
	all, err := repo.ListAPIKeys()
	require.NoError(t, err)
	owned, err := repo.ListAPIKeysByUser(user.ID)
	require.NoError(t, err)
	require.Contains(t, all, byID)
	require.Equal(t, []*APIKeyRecord{byID}, owned)
	_, err = repo.db.Exec(`UPDATE api_keys SET allowed_models = ? WHERE id = ?`, "broken JSON", key.ID)
	require.NoError(t, err)
	byID, err = repo.GetAPIKey(key.ID)
	require.NoError(t, err)
	require.Nil(t, byID.AllowedModels)
	require.NoError(t, repo.DeleteAPIKey(key.ID))
	_, err = repo.GetAPIKey(key.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	owned, err = repo.ListAPIKeysByUser(user.ID)
	require.NoError(t, err)
	require.NotNil(t, owned)
	require.Empty(t, owned)
}

func checkChannelReadProjections(t *testing.T, repo *Repository, suffix string) {
	t.Helper()
	channel := &ChannelRecord{Name: "scan-" + suffix, Type: model.ProviderOpenAI, Models: []string{"first"}, ModelMapping: map[string]string{"first": "upstream"}, Protocols: []string{"openai_chat"}, MaxConcurrency: 3, QueueLimit: 7, QueueWaitSeconds: 11}
	require.NoError(t, repo.CreateChannel(channel))
	byID, err := scanChannel(repo.db.QueryRow(`SELECT `+channelColumns+` FROM channels WHERE id = ?`, channel.ID))
	require.NoError(t, err)
	listed, err := repo.ListChannels()
	require.NoError(t, err)
	require.Contains(t, listed, byID)
	require.Equal(t, []string{"first"}, byID.Models)
	require.Equal(t, map[string]string{"first": "upstream"}, byID.ModelMapping)
	require.Equal(t, []string{"openai_chat"}, byID.Protocols)
	_, err = repo.db.Exec(`UPDATE channels SET models = 'null', model_mapping = ?, protocols = ? WHERE id = ?`, "broken JSON", "", channel.ID)
	require.NoError(t, err)
	byID, err = scanChannel(repo.db.QueryRow(`SELECT `+channelColumns+` FROM channels WHERE id = ?`, channel.ID))
	require.NoError(t, err)
	require.Nil(t, byID.Models)
	require.Nil(t, byID.ModelMapping)
	require.Nil(t, byID.Protocols)
	_, err = repo.db.Exec(`UPDATE channels SET model_capabilities = ? WHERE id = ?`, "broken JSON", channel.ID)
	require.NoError(t, err)
	_, err = repo.ListChannels()
	require.ErrorContains(t, err, "decode channel capabilities")
	require.NoError(t, repo.DeleteChannel(channel.ID))
}

func checkPriceReadProjections(t *testing.T, repo *Repository, suffix string) {
	t.Helper()
	price := &ModelPriceRecord{Model: "scan-" + suffix, GroupName: "default", PromptPrice: 1.25, CompletionPrice: 7.5, CacheReadPrice: 0.3, Currency: "CNY", OffPeakEnabled: true, WeekendAllDay: false}
	require.NoError(t, repo.SaveModelPrice(price))
	_, err := repo.db.Exec(`UPDATE model_prices SET weekend_all_day = 2 WHERE model = ?`, price.Model)
	require.NoError(t, err)
	exact, err := repo.GetModelPriceExact(price.Model, "default")
	require.NoError(t, err)
	listed, err := repo.ListModelPrices()
	require.NoError(t, err)
	require.Contains(t, listed, exact)
	require.Equal(t, 1.25, exact.PromptPrice)
	require.Equal(t, 7.5, exact.CompletionPrice)
	require.Equal(t, 0.3, exact.CacheReadPrice)
	require.True(t, exact.OffPeakEnabled)
	require.False(t, exact.WeekendAllDay)
	_, err = repo.GetModelPriceExact(price.Model+"-missing", "default")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = repo.GetAPIKeyContext(context.Background(), -1)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
