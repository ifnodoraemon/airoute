package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

func setupModelDeleteTest(t *testing.T) (*storage.Repository, *router.Dispatcher, *controlplane.Synchronizer, *gin.Engine, string) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	require.NoError(t, repo.EnsureDefaultAdmin("admin", "test-delete-password"))
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	handler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	token, err := controlplane.GenerateAdminToken("admin", "admin", time.Minute)
	require.NoError(t, err)
	return repo, dispatcher, sync, api.SetupRouter(dispatcher, handler), token
}

func deleteModelRequest(engine http.Handler, target, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// Exercise the registered HTTP routes: invoking the handler directly cannot
// catch encoded slashes being decoded before Gin matches a :model parameter.
func TestAPI_DeleteModelRouteIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		model  string
		legacy bool
	}{
		{model: "gpt-simple", legacy: true},
		{model: "aliyun/glm-5.3"},
		{model: "vendor/team/模型 +%2F?#&"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			repo, dispatcher, sync, engine, token := setupModelDeleteTest(t)
			for _, name := range []string{"primary", "backup"} {
				require.NoError(t, repo.CreateChannel(&storage.ChannelRecord{
					Name: name, Type: "openai", BaseURL: "http://upstream.invalid/v1",
					Models:       []string{tc.model, "keep-model"},
					ModelMapping: map[string]string{tc.model: "upstream-model", "keep-model": "keep-upstream"},
					Priority:     1, Weight: 10, Status: "active",
				}))
			}
			require.NoError(t, repo.SetModelFallback(tc.model, "keep-model", true))
			require.NoError(t, repo.SetModelFallback("keep-model", "keep-backup", true))
			require.NoError(t, sync.ReloadAndBroadcast(context.Background(), "test_seed"))
			require.Len(t, dispatcher.GetChannelsForModel(tc.model), 2)

			target := "/api/v1/admin/models/routes?" + url.Values{"model": {tc.model}}.Encode()
			if tc.legacy {
				target = "/api/v1/admin/models/routes/" + url.PathEscape(tc.model)
			}
			w := deleteModelRequest(engine, target, token)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Empty(t, dispatcher.GetChannelsForModel(tc.model))
			require.Len(t, dispatcher.GetChannelsForModel("keep-model"), 2)
			channels, err := repo.ListChannels()
			require.NoError(t, err)
			require.Len(t, channels, 2)
			for _, ch := range channels {
				require.Equal(t, []string{tc.model, "keep-model"}, ch.Models)
				require.Equal(t, map[string]string{tc.model: "upstream-model", "keep-model": "keep-upstream"}, ch.ModelMapping)
			}
			fallbacks, err := repo.GetModelFallbacks()
			require.NoError(t, err)
			require.Equal(t, map[string]string{"keep-model": "keep-backup"}, fallbacks)
		})
	}
}

func TestAPI_DeletePricingIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		model  string
		legacy bool
	}{
		{model: "gpt-simple", legacy: true},
		{model: "aliyun/glm-5.3"},
		{model: "vendor/team/模型 +%2F?#&"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			repo, _, _, engine, token := setupModelDeleteTest(t)
			group := "vip +&/组"
			for _, g := range []string{"default", group} {
				require.NoError(t, repo.SaveModelPrice(&storage.ModelPriceRecord{
					Model: tc.model, GroupName: g, PromptPrice: 1, Currency: "CNY",
				}))
			}
			target := "/api/v1/admin/pricing?" + url.Values{"model": {tc.model}, "group": {group}}.Encode()
			if tc.legacy {
				target = "/api/v1/admin/pricing/" + url.PathEscape(tc.model) + "?" + url.Values{"group": {group}}.Encode()
			}
			w := deleteModelRequest(engine, target, token)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			_, err := repo.GetModelPriceExact(tc.model, group)
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = repo.GetModelPriceExact(tc.model, "default")
			require.NoError(t, err, "other groups must retain their pricing")
		})
	}
}

func TestAPI_DeleteModelRequiresAdminAndIdentifier(t *testing.T) {
	repo, _, _, engine, token := setupModelDeleteTest(t)
	require.NoError(t, repo.CreateUser(&storage.UserRecord{Username: "reader", Role: "user", Status: "active"}))
	userToken, err := controlplane.GenerateAdminToken("reader", "user", time.Minute)
	require.NoError(t, err)
	for _, path := range []string{"/api/v1/admin/models/routes", "/api/v1/admin/pricing"} {
		t.Run(path, func(t *testing.T) {
			w := deleteModelRequest(engine, path+"?model=vendor%2Fmodel", "")
			require.Equal(t, http.StatusUnauthorized, w.Code)
			w = deleteModelRequest(engine, path+"?model=vendor%2Fmodel", userToken)
			require.Equal(t, http.StatusForbidden, w.Code)
			for _, query := range []string{"", "?model=", "?model=%20%20"} {
				w = deleteModelRequest(engine, path+query, token)
				require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			}
		})
	}
}

func TestAPI_IndependentRouteLifecycle(t *testing.T) {
	repo, dispatcher, sync, engine, token := setupModelDeleteTest(t)
	ch := &storage.ChannelRecord{Name: "capabilities", Type: "openai", BaseURL: "http://upstream.invalid",
		Models: []string{"upstream-a", "upstream-b"}, Priority: 1, Weight: 10, Status: "active"}
	require.NoError(t, repo.CreateChannel(ch))
	before, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.NoError(t, sync.ReloadAndBroadcast(context.Background(), "test_seed"))
	require.Len(t, dispatcher.GetChannelsForModel("upstream-a"), 1, "capabilities initially generate automatic routes")

	save := func(name string) {
		body, err := json.Marshal(controlplane.UpdateModelRouteRequest{
			Model: name, ProviderUpdates: []controlplane.ModelProviderUpdate{{
				ChannelID: ch.ID, Priority: 3, Weight: 21, MappedModel: "upstream-a",
			}},
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/routes", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	save("public/alias")
	bound := dispatcher.GetChannelsForModel("public/alias")
	require.Len(t, bound, 1)
	require.Equal(t, 21, bound[0].Weight)
	require.Equal(t, 3, bound[0].Priority)
	require.Equal(t, "upstream-a", bound[0].GetUpstreamModel("public/alias"))
	require.Equal(t, 10, dispatcher.GetChannelsForModel("upstream-b")[0].Weight)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/routes/batch-delete", bytes.NewBufferString(`{"models":["public/alias","upstream-a"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Empty(t, dispatcher.GetChannelsForModel("public/alias"))
	require.Empty(t, dispatcher.GetChannelsForModel("upstream-a"))
	after, err := repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "single and batch route operations must not touch the channel")

	// A second replica/restarted process must load the same suppression policy.
	restarted := router.NewDispatcher(nil)
	require.NoError(t, controlplane.NewSynchronizer(repo, restarted).ReloadFromDB())
	require.Empty(t, restarted.GetChannelsForModel("upstream-a"))
	require.Len(t, restarted.GetChannelsForModel("upstream-b"), 1)
	for _, route := range restarted.GetModelRoutes() {
		require.NotContains(t, []string{"public/alias", "upstream-a"}, route.Model)
	}
	save("public/alias")
	require.Len(t, dispatcher.GetChannelsForModel("public/alias"), 1, "explicit creation restores a route")
	after, err = repo.GetChannel(ch.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
