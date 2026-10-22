package api

import (
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"net/http/httptest"
	"testing"
)

func TestMultiImageGroupDiscount(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{Model: "review-images", GroupName: "default", FixedPrice: 10, Currency: "CNY"}); err != nil {
		t.Fatal(err)
	}
	oldEngine := billing.GlobalEngine
	oldLogger := storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	storage.GlobalAsyncLogger = nil
	defer func() { billing.GlobalEngine = oldEngine; storage.GlobalAsyncLogger = oldLogger }()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-review-synthetic", GroupName: "vip"})
	got := RecordUsage(c, AuditRecordParams{Model: "review-images", StatusCode: 200, BillableUnits: 2})
	if got != 16 {
		t.Fatalf("2 images at 10 CNY each with vip 0.8 multiplier should cost 16, observed %v", got)
	}
}
