package controlplane

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newSkillTestHandler(t *testing.T) (*AdminHandler, *storage.Repository, storage.ArtifactStorage) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	handler := NewAdminHandler(repo, nil, nil)

	local, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("open local artifact storage: %v", err)
	}
	handler.storage = local
	return handler, repo, local
}

func seedSkill(t *testing.T, repo *storage.Repository, manifest string) *storage.SkillRecord {
	t.Helper()
	record := &storage.SkillRecord{
		ID: "demo-skill", Name: "Demo Skill", Description: "synthetic skill for ETag tests",
		Category: "custom", Tools: []string{"demo_tool"}, LoadingMode: "lazy",
		Manifest: manifest, Author: "Custom", Version: "1.0.0", Enabled: true,
	}
	if err := repo.SaveSkill(record); err != nil {
		t.Fatalf("save skill: %v", err)
	}
	return record
}

// downloadSkill invokes the download route through a real gin engine. Calling the
// handler directly would skip gin's final WriteHeaderNow, so a 304 set with
// c.Status would be recorded as 200 and the revalidation assertions would lie.
func downloadSkill(t *testing.T, handler *AdminHandler, id, ifNoneMatch string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/v1/admin/skills/:id/download", handler.DownloadSkillZip)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/skills/"+id+"/download", nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	engine.ServeHTTP(w, req)
	return w
}

func saveSkill(t *testing.T, handler *AdminHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/skills", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	handler.SaveSkill(c)
	return w
}

// skillManifestFromZip reads <id>/SKILL.md out of a downloaded bundle.
func skillManifestFromZip(t *testing.T, body []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("download is not a zip archive: %v", err)
	}
	for _, file := range reader.File {
		if !strings.HasSuffix(file.Name, "/SKILL.md") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open SKILL.md in bundle: %v", err)
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read SKILL.md in bundle: %v", err)
		}
		return string(content)
	}
	t.Fatalf("bundle has no SKILL.md: %v", reader.File)
	return ""
}

// #20: the artifact identity must include the bundle content, not just id+version.
// Before this, editing a manifest without bumping the version produced the same
// ETag (so browsers kept a stale 304) and the handler reused the previously cached
// zip, so downloads never changed.
func TestSkillDownloadETagTracksContentNotVersion(t *testing.T) {
	handler, repo, stor := newSkillTestHandler(t)
	seedSkill(t, repo, "# skill manifest v1")

	first := downloadSkill(t, handler, "demo-skill", "")
	if first.Code != http.StatusOK {
		t.Fatalf("first download = %d: %s", first.Code, first.Body.String())
	}
	firstETag := first.Header().Get("ETag")
	if firstETag == "" {
		t.Fatal("download response has no ETag")
	}
	if cacheControl := first.Header().Get("Cache-Control"); !strings.Contains(cacheControl, "max-age=0") || !strings.Contains(cacheControl, "must-revalidate") {
		t.Errorf("Cache-Control = %q, want the response to revalidate", cacheControl)
	}
	if manifest := skillManifestFromZip(t, first.Body.Bytes()); manifest != "# skill manifest v1" {
		t.Fatalf("first bundle SKILL.md = %q", manifest)
	}

	before, err := repo.GetSkill("demo-skill")
	if err != nil {
		t.Fatalf("read seeded skill: %v", err)
	}
	cachedKey, err := skillArtifactKey(before)
	if err != nil {
		t.Fatalf("compute artifact key: %v", err)
	}
	if exists, _ := stor.Exists(context.Background(), cachedKey); !exists {
		t.Fatal("download did not cache the content-addressed artifact")
	}

	// Identical content revalidates to 304, including the weak form browsers send.
	revalidated := downloadSkill(t, handler, "demo-skill", firstETag)
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("identical content revalidation = %d, want 304: %s", revalidated.Code, revalidated.Body.String())
	}
	weak := downloadSkill(t, handler, "demo-skill", "W/"+firstETag)
	if weak.Code != http.StatusNotModified {
		t.Fatalf("weak ETag revalidation = %d, want 304", weak.Code)
	}

	// Edit the content without changing the version.
	edited := saveSkill(t, handler, `{"id":"demo-skill","name":"Demo Skill","description":"synthetic skill for ETag tests","category":"custom","tools":["demo_tool"],"manifest":"# skill manifest v2","version":"1.0.0","enabled":true}`)
	if edited.Code != http.StatusOK {
		t.Fatalf("edit skill = %d: %s", edited.Code, edited.Body.String())
	}
	if exists, _ := stor.Exists(context.Background(), cachedKey); exists {
		t.Error("the stale artifact was left behind after the edit")
	}

	second := downloadSkill(t, handler, "demo-skill", "")
	if second.Code != http.StatusOK {
		t.Fatalf("download after edit = %d: %s", second.Code, second.Body.String())
	}
	if secondETag := second.Header().Get("ETag"); secondETag == firstETag {
		t.Fatalf("ETag did not change after a content edit: %q", secondETag)
	}
	if manifest := skillManifestFromZip(t, second.Body.Bytes()); manifest != "# skill manifest v2" {
		t.Fatalf("second bundle was served from the stale cache: SKILL.md = %q", manifest)
	}

	// The old ETag must no longer revalidate: the browser has to fetch the new body.
	stale := downloadSkill(t, handler, "demo-skill", firstETag)
	if stale.Code != http.StatusOK {
		t.Fatalf("stale ETag revalidation = %d, want a fresh 200", stale.Code)
	}
	if manifest := skillManifestFromZip(t, stale.Body.Bytes()); manifest != "# skill manifest v2" {
		t.Fatalf("stale ETag received %q, want the current content", manifest)
	}

	// The current ETag still revalidates.
	if current := downloadSkill(t, handler, "demo-skill", second.Header().Get("ETag")); current.Code != http.StatusNotModified {
		t.Fatalf("current ETag revalidation = %d, want 304", current.Code)
	}
}

// The artifact key and ETag are derived from the same bytes, so a hash change
// changes both.
func TestSkillArtifactFingerprintIsStableAndContentSensitive(t *testing.T) {
	var first, second storage.SkillRecord
	raw := `{"id":"demo-skill","name":"Demo Skill","description":"d","category":"custom","tools":["demo_tool"],"manifest":"v1"}`
	if err := json.Unmarshal([]byte(raw), &first); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	second = first
	second.Manifest = "v2"

	firstHash, err := skillArtifactFingerprint(&first)
	if err != nil {
		t.Fatalf("hash first: %v", err)
	}
	again, err := skillArtifactFingerprint(&first)
	if err != nil {
		t.Fatalf("hash first again: %v", err)
	}
	if firstHash != again {
		t.Fatalf("fingerprint is not stable: %q != %q", firstHash, again)
	}

	secondHash, err := skillArtifactFingerprint(&second)
	if err != nil {
		t.Fatalf("hash second: %v", err)
	}
	if firstHash == secondHash {
		t.Fatal("a manifest edit did not change the content fingerprint")
	}
}
