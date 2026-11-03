package tests

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReviewMalformedWritesDoNotCreateOrEnableResources(t *testing.T) {
	a := newClosureApplication(t)
	a.call("POST", "/api/v1/admin/users", a.admin, `{"username":"review-owner","password":"review-password"}`, 200)
	login := a.call("POST", "/api/v1/auth/login", "", `{"username":"review-owner","password":"review-password"}`, 200)
	token := closureData(t, login)["token"].(string)
	user, err := a.repo.GetUserByUsername("review-owner")
	require.NoError(t, err)
	before, err := a.repo.ListAPIKeysByUser(user.ID)
	require.NoError(t, err)
	for _, body := range []string{`{`, `{"rpm":"invalid"}`, `{"allowed_models":"model"}`} {
		a.call("POST", "/api/v1/admin/keys", token, body, 400)
	}
	a.call("POST", "/api/v1/user/keys", token, `{}`, 404)
	after, err := a.repo.ListAPIKeysByUser(user.ID)
	require.NoError(t, err)
	require.Len(t, after, len(before))

	a.call("POST", "/api/v1/admin/skills", a.admin, `{"id":"review-skill","name":"Review","description":"Test","enabled":false}`, 200)
	a.call("POST", "/api/v1/admin/mcp/servers", a.admin, `{"id":"review-mcp","name":"Review","transport":"http","endpoint":"http://127.0.0.1:1/mcp","enabled":false}`, 200)
	for _, body := range []string{`{`, `{}`, `null`, `{"enabled":"false"}`} {
		a.call("POST", "/api/v1/admin/skills/review-skill/toggle", a.admin, body, 400)
		a.call("POST", "/api/v1/admin/mcp/servers/review-mcp/toggle", a.admin, body, 400)
	}
	for _, body := range []string{`{"enabled":true}`, `{"enabled":false}`, `{"enabled":false}`} {
		a.call("POST", "/api/v1/admin/skills/review-skill/toggle", a.admin, body, 200)
		a.call("POST", "/api/v1/admin/mcp/servers/review-mcp/toggle", a.admin, body, 200)
	}
	skill, err := a.repo.GetSkill("review-skill")
	require.NoError(t, err)
	require.False(t, skill.Enabled)
	server, err := a.repo.GetMCPServer("review-mcp")
	require.NoError(t, err)
	require.False(t, server.Enabled)
}

func TestReviewSkillBundleContainsOnlyUsableArtifacts(t *testing.T) {
	a := newClosureApplication(t)
	a.call("POST", "/api/v1/admin/skills", a.admin, `{"id":"review-bundle","name":"Review","description":"Test","manifest":"# Real SOP","enabled":true}`, 200)
	w := a.call("GET", "/api/v1/skills/review-bundle/download", "", "", 200)
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	var names []string
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	require.ElementsMatch(t, []string{"review-bundle/SKILL.md", "review-bundle/references/metadata.json"}, names)
}
