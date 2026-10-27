package storage_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newOrgRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db)
}

func mustCreateOrg(t *testing.T, repo *storage.Repository, org *storage.OrgRecord) *storage.OrgRecord {
	t.Helper()
	if err := repo.CreateOrg(org); err != nil {
		t.Fatalf("create org %s: %v", org.Code, err)
	}
	return org
}

func TestDefaultOrganizationExists(t *testing.T) {
	repo := newOrgRepo(t)
	root, err := repo.GetOrgByCode("default")
	if err != nil {
		t.Fatalf("default org: %v", err)
	}
	if root.Level != storage.OrgLevelUnit || root.Depth != 0 || root.Status != "active" {
		t.Fatalf("unexpected default org: %+v", root)
	}
	if want := fmt.Sprintf("/%d/", root.ID); root.Path != want {
		t.Fatalf("default path = %q, want %q", root.Path, want)
	}
}

func TestCreateOrgTreeComputesPathsAndDepths(t *testing.T) {
	repo := newOrgRepo(t)
	unit := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "unit", Name: "宁夏移动", Code: "nx"})
	if want := fmt.Sprintf("/%d/", unit.ID); unit.Path != want || unit.Depth != 0 {
		t.Fatalf("unit path=%q depth=%d", unit.Path, unit.Depth)
	}
	dept := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "department", Name: "算力运营", Code: "nx-compute", ParentID: unit.ID})
	if want := unit.Path + fmt.Sprintf("%d/", dept.ID); dept.Path != want || dept.Depth != 1 {
		t.Fatalf("department path=%q depth=%d, want %q/1", dept.Path, dept.Depth, want)
	}
	project := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "project", Name: "创新业务", Code: "nx-compute-innov", ParentID: dept.ID})
	if want := dept.Path + fmt.Sprintf("%d/", project.ID); project.Path != want || project.Depth != 2 {
		t.Fatalf("project path=%q depth=%d, want %q/2", project.Path, project.Depth, want)
	}

	children, err := repo.ListOrgChildren(unit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != dept.ID {
		t.Fatalf("children = %+v", children)
	}
	all, err := repo.ListOrgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 4 {
		t.Fatalf("expected default + unit + department + project, got %d", len(all))
	}
}

func TestCreateOrgValidatesHierarchyAndUniqueness(t *testing.T) {
	repo := newOrgRepo(t)
	unit := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "unit", Name: "Unit", Code: "u1"})
	dept := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "department", Name: "Dept", Code: "d1", ParentID: unit.ID})

	cases := []struct {
		name string
		org  *storage.OrgRecord
	}{
		{"unit with parent", &storage.OrgRecord{Level: "unit", Name: "Bad", Code: "bad1", ParentID: unit.ID}},
		{"department without parent", &storage.OrgRecord{Level: "department", Name: "Bad", Code: "bad2"}},
		{"project under unit", &storage.OrgRecord{Level: "project", Name: "Bad", Code: "bad3", ParentID: unit.ID}},
		{"unknown level", &storage.OrgRecord{Level: "team", Name: "Bad", Code: "bad4"}},
		{"missing name", &storage.OrgRecord{Level: "unit", Name: " ", Code: "bad5"}},
		{"duplicate code", &storage.OrgRecord{Level: "department", Name: "Dup", Code: "d1", ParentID: unit.ID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := repo.CreateOrg(tc.org); err == nil {
				t.Fatalf("expected validation error for %+v", tc.org)
			}
		})
	}

	if err := repo.UpdateOrg(unit.ID, "", "archived"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateOrg(&storage.OrgRecord{Level: "department", Name: "Child", Code: "d2", ParentID: unit.ID}); err == nil {
		t.Fatal("archived parents must not accept new children")
	}
	_ = dept
}

func TestMoveOrgRewritesSubtreePaths(t *testing.T) {
	repo := newOrgRepo(t)
	unitA := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "unit", Name: "Unit A", Code: "ua"})
	unitB := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "unit", Name: "Unit B", Code: "ub"})
	dept := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "department", Name: "Dept", Code: "dept", ParentID: unitA.ID})
	project := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "project", Name: "Project", Code: "proj", ParentID: dept.ID})

	if err := repo.MoveOrg(dept.ID, unitB.ID); err != nil {
		t.Fatalf("move department: %v", err)
	}
	movedDept, err := repo.GetOrg(dept.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := unitB.Path + fmt.Sprintf("%d/", dept.ID); movedDept.Path != want {
		t.Fatalf("department path = %q, want %q", movedDept.Path, want)
	}
	movedProject, err := repo.GetOrg(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := movedDept.Path + fmt.Sprintf("%d/", project.ID); movedProject.Path != want {
		t.Fatalf("project path = %q, want %q", movedProject.Path, want)
	}
	if movedProject.Depth != 2 {
		t.Fatalf("project depth = %d, want 2", movedProject.Depth)
	}

	// Moving an ancestor under its own descendant must be rejected.
	if err := repo.MoveOrg(unitB.ID, dept.ID); err == nil {
		t.Fatal("expected a cycle rejection")
	}
	if err := repo.MoveOrg(unitA.ID, unitB.ID); err == nil {
		t.Fatal("a root unit cannot be moved")
	}
}

func TestOrgMembersAndAncestors(t *testing.T) {
	repo := newOrgRepo(t)
	unit := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "unit", Name: "Unit", Code: "mu"})
	dept := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "department", Name: "Dept", Code: "md", ParentID: unit.ID})
	project := mustCreateOrg(t, repo, &storage.OrgRecord{Level: "project", Name: "Project", Code: "mp", ParentID: dept.ID})

	user := &storage.UserRecord{Username: "org-member", PasswordHash: "x"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repo.AddOrgMember(project.ID, user.ID, "admin"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	members, err := repo.ListOrgMembers(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].UserID != user.ID || members[0].Role != "admin" || members[0].Username != user.Username {
		t.Fatalf("members = %+v", members)
	}
	role, err := repo.OrgMemberRole(project.ID, user.ID)
	if err != nil || role != "admin" {
		t.Fatalf("role=%q err=%v", role, err)
	}
	orgs, err := repo.ListUserOrgs(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != project.ID {
		t.Fatalf("user orgs = %+v", orgs)
	}

	chain, err := repo.OrgAncestors(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 || chain[0].ID != unit.ID || chain[1].ID != dept.ID || chain[2].ID != project.ID {
		t.Fatalf("ancestors = %+v", chain)
	}

	if err := repo.RemoveOrgMember(project.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.OrgMemberRole(project.ID, user.ID); err == nil {
		t.Fatal("expected membership to be gone")
	}
}

func TestOrgUserOrgIDFallsBackToDefault(t *testing.T) {
	repo := newOrgRepo(t)
	root, err := repo.DefaultOrgID()
	if err != nil {
		t.Fatal(err)
	}
	user := &storage.UserRecord{Username: "no-org", PasswordHash: "x"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	orgID, err := repo.UserOrgID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if orgID != root {
		t.Fatalf("user org id = %d, want default %d", orgID, root)
	}
	if strings.TrimSpace(fmt.Sprintf("%d", orgID)) == "" {
		t.Fatal("unreachable")
	}
}
