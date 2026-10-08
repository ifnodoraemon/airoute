package tests

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStorage_CRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "airoute-storage-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	stor, err := storage.NewLocalStorage(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "local", stor.Driver())

	ctx := context.Background()
	key := storage.FormatSkillKey("test-skill")
	payload := []byte("PK\x03\x04mock_zip_content")

	// 1. Initial exists should be false
	exists, err := stor.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)

	// 2. Put
	err = stor.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)), "application/zip")
	assert.NoError(t, err)

	// 3. Exists should be true
	exists, err = stor.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// 4. Get
	rc, size, err := stor.Get(ctx, key)
	require.NoError(t, err)
	defer rc.Close()
	assert.Equal(t, int64(len(payload)), size)
	gotBytes, err := io.ReadAll(rc)
	assert.NoError(t, err)
	assert.Equal(t, payload, gotBytes)

	// 5. Presigned URL is empty for local
	url, err := stor.GetDownloadURL(ctx, key, 10*time.Minute)
	assert.NoError(t, err)
	assert.Empty(t, url)

	// 6. Delete
	err = stor.Delete(ctx, key)
	assert.NoError(t, err)
	exists, _ = stor.Exists(ctx, key)
	assert.False(t, exists)
}

func TestS3Storage_WithMockRustFS(t *testing.T) {
	mockObjects := make(map[string][]byte)
	var mu sync.Mutex

	// Mock S3/RustFS HTTP Server verifying AWS SigV4 headers
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		authHeader := r.Header.Get("Authorization")
		amzDate := r.Header.Get("x-amz-date")
		contentSha := r.Header.Get("x-amz-content-sha256")

		path := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.SplitN(path, "/", 2)
		bucket := parts[0]
		key := ""
		if len(parts) > 1 {
			key = parts[1]
		}

		switch r.Method {
		case http.MethodPut:
			if key == "" {
				// Create Bucket
				w.WriteHeader(http.StatusOK)
				return
			}
			assert.NotEmpty(t, authHeader, "Authorization header must be present on S3 Put")
			assert.NotEmpty(t, amzDate, "x-amz-date must be present")
			assert.NotEmpty(t, contentSha, "x-amz-content-sha256 must be present")

			data, _ := io.ReadAll(r.Body)
			mockObjects[bucket+"/"+key] = data
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			assert.NotEmpty(t, authHeader, "Authorization header must be present on S3 Get")
			data, found := mockObjects[bucket+"/"+key]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)

		case http.MethodHead:
			if key == "" {
				// Bucket exists check
				w.WriteHeader(http.StatusOK)
				return
			}
			_, found := mockObjects[bucket+"/"+key]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)

		case http.MethodDelete:
			delete(mockObjects, bucket+"/"+key)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer s3Server.Close()

	s3Cfg := config.S3Config{
		Endpoint:  s3Server.URL,
		Bucket:    "airoute-skills",
		AccessKey: "rustfsadmin",
		SecretKey: "rustfssecret",
		Region:    "us-east-1",
		UseSSL:    false,
		PathStyle: true,
	}

	s3Stor, err := storage.NewS3Storage(s3Cfg)
	require.NoError(t, err)
	assert.Equal(t, "s3", s3Stor.Driver())

	ctx := context.Background()
	key := "skills/rustfs-demo/rustfs-demo.zip"
	mockData := []byte("PK\x03\x04RUSTFS_MOCK_BUNDLE")

	// 1. Put object
	err = s3Stor.Put(ctx, key, bytes.NewReader(mockData), int64(len(mockData)), "application/zip")
	assert.NoError(t, err)

	// 2. Exists
	exists, err := s3Stor.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// 3. Get
	rc, size, err := s3Stor.Get(ctx, key)
	require.NoError(t, err)
	defer rc.Close()
	assert.Equal(t, int64(len(mockData)), size)
	got, _ := io.ReadAll(rc)
	assert.Equal(t, mockData, got)

	// 4. Presigned Download URL (AWS SigV4 Query auth)
	dlURL, err := s3Stor.GetDownloadURL(ctx, key, 15*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, dlURL, s3Server.URL)
	assert.Contains(t, dlURL, "X-Amz-Algorithm=AWS4-HMAC-SHA256")
	assert.Contains(t, dlURL, "X-Amz-Signature=")
	assert.Contains(t, dlURL, "X-Amz-Expires=900")
	assert.Contains(t, dlURL, "rustfsadmin")

	// 5. Delete
	err = s3Stor.Delete(ctx, key)
	assert.NoError(t, err)
	exists, _ = s3Stor.Exists(ctx, key)
	assert.False(t, exists)
}

func TestStorageStatusAndSkillDownloadCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir, _ := os.MkdirTemp("", "airoute-integ-storage-*")
	defer os.RemoveAll(tmpDir)

	dbFile := filepath.Join(tmpDir, "test.db")
	db, err := storage.OpenDB(dbFile)
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)

	localStorageDir := filepath.Join(tmpDir, "artifacts")
	artifactStor, err := storage.NewLocalStorage(localStorageDir)
	require.NoError(t, err)
	adminHandler.SetArtifactStorage(artifactStor)

	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Test /api/v1/admin/storage/status
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/v1/admin/storage/status", nil)
	wStatus := httptest.NewRecorder()
	engine.ServeHTTP(wStatus, reqStatus)
	assert.Equal(t, http.StatusOK, wStatus.Code)
	assert.Contains(t, wStatus.Body.String(), `"driver":"local"`)

	// 2. Test downloading a skill - should dynamically create, return ZIP, and cache to storage
	reqDl := httptest.NewRequest(http.MethodGet, "/api/v1/skills/git-workflow/download", nil)
	wDl := httptest.NewRecorder()
	engine.ServeHTTP(wDl, reqDl)
	assert.Equal(t, http.StatusOK, wDl.Code)
	assert.Equal(t, "application/zip", wDl.Header().Get("Content-Type"))
	assert.True(t, wDl.Body.Len() > 0)

	// Wait briefly for asynchronous cache write
	time.Sleep(50 * time.Millisecond)

	// 3. Verify cached file in storage
	key := storage.FormatSkillKey("git-workflow")
	exists, err := artifactStor.Exists(context.Background(), key)
	assert.NoError(t, err)
	assert.True(t, exists, "Skill ZIP bundle should be cached in artifact storage")
}
