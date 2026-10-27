package governance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
	"github.com/ifnodoraemon/airoute/internal/notify"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type fakeNotifier struct {
	mu       sync.Mutex
	messages []notify.Message
	err      error
}

func (f *fakeNotifier) Name() string { return "fake" }
func (f *fakeNotifier) Send(_ context.Context, message notify.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, message)
	return f.err
}

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func waitForMessages(t *testing.T, notifier *fakeNotifier, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if notifier.count() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("notifier messages = %d, want %d", notifier.count(), want)
}

type budgetFixture struct {
	repo     *storage.Repository
	db       *storage.DB
	service  *Service
	notifier *fakeNotifier
	org      *storage.OrgRecord
}

func newBudgetFixture(t *testing.T) *budgetFixture {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.CreateBudgets(db); err != nil {
		t.Fatalf("create budget schema: %v", err)
	}
	repo := storage.NewRepository(db)
	unit := &storage.OrgRecord{Level: storage.OrgLevelUnit, Name: "单位", Code: "gov-unit"}
	if err := repo.CreateOrg(unit); err != nil {
		t.Fatal(err)
	}
	dept := &storage.OrgRecord{Level: storage.OrgLevelDepartment, Name: "部门", Code: "gov-dept", ParentID: unit.ID}
	if err := repo.CreateOrg(dept); err != nil {
		t.Fatal(err)
	}
	notifier := &fakeNotifier{}
	service := NewService(repo, budget.NewMemoryCounter(), notifier)
	return &budgetFixture{repo: repo, db: db, service: service, notifier: notifier, org: dept}
}

func (f *budgetFixture) addBudget(t *testing.T, revenue float64) *storage.BudgetRecord {
	t.Helper()
	rec := &storage.BudgetRecord{
		ScopeType: "org", ScopeID: f.org.ID, Period: "monthly",
		AmountUnits: storage.ToUnits(revenue), Action: "disable", WarnPercent: 80,
	}
	if err := f.repo.UpsertBudget(rec); err != nil {
		t.Fatalf("upsert budget: %v", err)
	}
	return rec
}

func TestRecordSpendEmitsEventsOnceAndCheckBlocks(t *testing.T) {
	fixture := newBudgetFixture(t)
	record := fixture.addBudget(t, 1000)
	subject := Subject{KeyID: 9, UserID: 7, OrgID: fixture.org.ID}
	ctx := context.Background()

	// 50%: an event is recorded, but it is informational (no notification).
	fixture.service.RecordSpend(ctx, subject, storage.ToUnits(500))
	events, err := fixture.repo.ListBudgetEvents(record.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Threshold != 50 {
		t.Fatalf("50%% events = %+v", events)
	}
	if fixture.notifier.count() != 0 {
		t.Fatalf("50%% must not notify: %d", fixture.notifier.count())
	}

	// 80%: notify and record; a repeated crossing must not notify twice.
	fixture.service.RecordSpend(ctx, subject, storage.ToUnits(300))
	waitForMessages(t, fixture.notifier, 1)
	fixture.service.RecordSpend(ctx, subject, storage.ToUnits(100))
	time.Sleep(100 * time.Millisecond)
	if fixture.notifier.count() != 1 {
		t.Fatalf("threshold 80 notified %d times", fixture.notifier.count())
	}

	// 100%: blocked for a disable rule.
	fixture.service.RecordSpend(ctx, subject, storage.ToUnits(100))
	waitForMessages(t, fixture.notifier, 2)
	decision, blocked, err := fixture.service.Check(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || decision.Allowed {
		t.Fatalf("expected a block, got %+v blocked=%v", decision, blocked)
	}
}

func TestCheckFailsOpenWhenRulesAreUnavailable(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// No CreateBudgets: the table does not exist, which is the pre-migration
	// state. The data plane must keep serving instead of failing closed on a
	// missing optional table.
	service := NewService(storage.NewRepository(db), budget.NewMemoryCounter(), &fakeNotifier{})
	decision, blocked, err := service.Check(context.Background(), Subject{KeyID: 9, UserID: 7})
	if err == nil {
		t.Fatal("expected the missing table to surface as an error")
	}
	if blocked || !decision.Allowed {
		t.Fatalf("missing rules must fail open: %+v blocked=%v", decision, blocked)
	}
}

func TestInvalidateRefreshesRules(t *testing.T) {
	fixture := newBudgetFixture(t)
	subject := Subject{KeyID: 9, UserID: 7, OrgID: fixture.org.ID}
	ctx := context.Background()

	// Warm the cache with no rules.
	if _, blocked, err := fixture.service.Check(ctx, subject); err != nil || blocked {
		t.Fatalf("initial check: %v %v", err, blocked)
	}
	fixture.addBudget(t, 1000)
	// Cached empty rules would hide the new budget until TTL.
	if _, blocked, err := fixture.service.Check(ctx, subject); err != nil || blocked {
		t.Fatalf("cached check should not see the new rule yet: %v %v", err, blocked)
	}
	fixture.service.Invalidate()
	if _, blocked, err := fixture.service.Check(ctx, subject); err != nil || blocked {
		t.Fatalf("new rule should still allow an unused budget: %v %v", err, blocked)
	}
}
