package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/require"
)

type notificationAPIFixture struct {
	engine *gin.Engine
	tokens map[string]string
}

func newNotificationAPIFixture(t *testing.T) *notificationAPIFixture {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	handler := controlplane.NewAdminHandler(repo, nil, nil)
	f := &notificationAPIFixture{engine: SetupRouter(router.NewDispatcher(nil), handler), tokens: map[string]string{}}
	for _, name := range []string{"admin", "alice", "bob"} {
		user := &storage.UserRecord{Username: name, Role: "user", Status: "active", PasswordHash: "test"}
		if name == "admin" {
			user.Role = "admin"
		}
		require.NoError(t, repo.CreateUser(user))
		f.tokens[name], err = controlplane.GenerateAdminToken(user, time.Hour)
		require.NoError(t, err)
	}
	return f
}

func (f *notificationAPIFixture) request(t *testing.T, user, method, path string, body any, status int) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+f.tokens[user])
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	require.Equal(t, status, w.Code, w.Body.String())
	return w.Body.Bytes()
}

func TestNotificationAPIAuthorizationAndDelivery(t *testing.T) {
	f := newNotificationAPIFixture(t)
	for _, path := range []string{"/api/v1/user/notifications", "/api/v1/user/notifications/unread-count"} {
		f.request(t, "", "GET", path, nil, 401)
	}
	for _, path := range []string{"/api/v1/user/notifications/anything/read", "/api/v1/user/notifications/read-all", "/api/v1/admin/notifications"} {
		f.request(t, "", "POST", path, nil, 401)
	}
	f.request(t, "alice", "POST", "/api/v1/admin/notifications", nil, 403)
	f.request(t, "alice", "POST", "/api/v1/admin/announcements", nil, 403)
	f.request(t, "alice", "GET", "/api/v1/admin/announcements", nil, 403)
	body := map[string]any{"request_id": "first", "recipient_username": " ALICE ", "title": "专属通知", "content": "只给 Alice\n保留换行", "level": "warning", "user_id": 3, "id": "spoofed"}
	data := f.request(t, "admin", "POST", "/api/v1/admin/notifications", body, 200)
	require.NotContains(t, string(data), "spoofed")
	f.request(t, "admin", "POST", "/api/v1/admin/notifications", body, 200)
	f.request(t, "bob", "POST", "/api/v1/user/notifications/msg_first/read", nil, 404)
	for _, user := range []string{"alice", "bob"} {
		data = f.request(t, user, "GET", "/api/v1/user/notifications?kind=message", nil, 200)
		var result struct{ Data storage.NotificationPage }
		require.NoError(t, json.Unmarshal(data, &result))
		if user == "alice" {
			require.EqualValues(t, 1, result.Data.Total)
			require.Equal(t, "只给 Alice\n保留换行", result.Data.Items[0].Content)
		} else {
			require.Zero(t, result.Data.Total)
			require.Empty(t, result.Data.Items)
			require.NotContains(t, string(data), "专属通知")
		}
	}
	f.request(t, "alice", "POST", "/api/v1/user/notifications/msg_first/read", nil, 200)
	data = f.request(t, "alice", "GET", "/api/v1/user/notifications/unread-count", nil, 200)
	require.JSONEq(t, `{"code":0,"data":{"unread_count":0,"announcement_unread_count":0,"message_unread_count":0}}`, string(data))
}

func TestAnnouncementTimelineWithoutBanner(t *testing.T) {
	f := newNotificationAPIFixture(t)
	body := map[string]any{"title": "产品更新", "content": "新功能说明", "level": "info", "status": "published", "request_id": "post1", "show_banner": false}
	f.request(t, "admin", "POST", "/api/v1/admin/announcements", body, 200)
	data := f.request(t, "", "GET", "/api/v1/announcements/banner", nil, 200)
	require.JSONEq(t, `{"code":0,"data":null}`, string(data))
	for _, user := range []string{"alice", "bob"} {
		data = f.request(t, user, "GET", "/api/v1/user/notifications?kind=announcement", nil, 200)
		var result struct{ Data storage.NotificationPage }
		require.NoError(t, json.Unmarshal(data, &result))
		require.EqualValues(t, 1, result.Data.Total)
		require.Equal(t, "产品更新", result.Data.Items[0].Title)
		f.request(t, user, "POST", "/api/v1/user/notifications/read-all", map[string]any{"through_sequence": result.Data.LatestSequence, "kind": "announcement"}, 200)
	}
	body["show_banner"] = true
	f.request(t, "admin", "PUT", "/api/v1/admin/announcements/ann_post1", body, 200)
	data = f.request(t, "", "GET", "/api/v1/announcements/banner", nil, 200)
	require.Contains(t, string(data), "产品更新")
	data = f.request(t, "alice", "GET", "/api/v1/user/notifications?kind=announcement&unread=true", nil, 200)
	var result struct{ Data storage.NotificationPage }
	require.NoError(t, json.Unmarshal(data, &result))
	require.Zero(t, result.Data.Total, "changing banner display must not redeliver a publication")
}

func TestNotificationAPIValidation(t *testing.T) {
	f := newNotificationAPIFixture(t)
	for _, query := range []string{"page=-1", "page_size=101", "page=no", "unread=yes", "kind=invalid"} {
		f.request(t, "alice", "GET", "/api/v1/user/notifications?"+query, nil, 400)
	}
	f.request(t, "alice", "POST", "/api/v1/user/notifications/read-all", map[string]any{"through_sequence": 1}, 400)
	for _, bad := range []map[string]any{
		{"title": " ", "content": "内容", "enabled": true},
		{"title": strings.Repeat("字", 121), "content": "内容"},
		{"title": "标题", "content": strings.Repeat("字", 10001)},
		{"title": "标题", "content": "内容", "level": "invalid"},
		{"content": strings.Repeat("a", 70000)},
	} {
		bad["request_id"], bad["status"] = "invalid", "published"
		f.request(t, "admin", "POST", "/api/v1/admin/announcements", bad, http.StatusBadRequest)
	}
	f.request(t, "admin", "POST", "/api/v1/admin/notifications", map[string]any{"request_id": "valid", "recipient_username": "missing", "title": "标题", "content": "内容"}, 404)
	f.request(t, "admin", "POST", "/api/v1/admin/notifications", map[string]any{"request_id": "valid", "title": "标题", "content": "内容"}, 400)
}

func TestAnnouncementManagementAndRemovedEndpoints(t *testing.T) {
	f := newNotificationAPIFixture(t)
	body := map[string]any{"request_id": "managed", "title": "草稿", "content": "正文", "level": "info", "status": "draft", "show_banner": true}
	f.request(t, "admin", "POST", "/api/v1/admin/announcements", body, 200)
	data := f.request(t, "admin", "GET", "/api/v1/admin/announcements?status=draft", nil, 200)
	require.Contains(t, string(data), "草稿")
	body["status"] = "published"
	f.request(t, "alice", "PUT", "/api/v1/admin/announcements/ann_managed", body, 403)
	f.request(t, "admin", "PUT", "/api/v1/admin/announcements/ann_managed", body, 200)
	data = f.request(t, "alice", "GET", "/api/v1/user/notifications?kind=announcement", nil, 200)
	require.Contains(t, string(data), "草稿")
	body["title"] = "已更正的公告"
	f.request(t, "admin", "PUT", "/api/v1/admin/announcements/ann_managed", body, 200)
	data = f.request(t, "alice", "GET", "/api/v1/user/notifications?kind=announcement", nil, 200)
	require.NotContains(t, string(data), "草稿")
	require.Contains(t, string(data), "已更正的公告")
	f.request(t, "alice", "DELETE", "/api/v1/admin/announcements/ann_managed", nil, 403)
	f.request(t, "admin", "DELETE", "/api/v1/admin/announcements/ann_managed", nil, 200)
	f.request(t, "admin", "GET", "/api/v1/admin/announcements/ann_managed", nil, 404)
	data = f.request(t, "alice", "GET", "/api/v1/user/notifications?kind=announcement", nil, 200)
	require.NotContains(t, string(data), "已更正的公告")
	for _, path := range []string{"/api/v1/announcement", "/api/v1/admin/announcement/config"} {
		f.request(t, "admin", "GET", path, nil, 404)
	}
	f.request(t, "admin", "POST", "/api/v1/admin/announcement", body, 404)
}
