package controlplane

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestSupplierCSVExportImportPreservesPricesAndIdentifiers(t *testing.T) {
	h, _ := newCostTestHandler(t)
	for _, model := range []string{"normal", "=formula", "'=literal"} {
		err := h.repo.UpsertSupplierCost(&storage.SupplierCostRecord{Channel: "@supplier", Model: model,
			InputCostUnitsPerMillion: 1, OutputCostUnitsPerMillion: 2123456789, Status: "disabled"})
		if err != nil {
			t.Fatal(err)
		}
	}
	exported := invokeCost(t, h.ExportSupplierCosts, "GET", "/export", "", nil)
	for range 2 {
		imported := invokeCost(t, h.ImportSupplierCosts, "POST", "/import", exported.Body.String(), nil)
		if imported.Code != http.StatusOK {
			t.Fatalf("import: %s", imported.Body.String())
		}
	}
	rows, err := h.repo.ListSupplierCosts()
	if err != nil || len(rows) != 3 {
		t.Fatalf("round trip created duplicates: rows=%v err=%v", rows, err)
	}
	for _, model := range []string{"normal", "=formula", "'=literal"} {
		row, err := h.repo.GetSupplierCost("@supplier", model)
		if err != nil || row.InputCostUnitsPerMillion != 1 || row.OutputCostUnitsPerMillion != 2123456789 || row.Status != "disabled" {
			t.Fatalf("round trip lost price or identity for %q: %+v err=%v", model, row, err)
		}
	}
}

func TestSupplierCSVRejectsInvalidFileWithoutPartialWrites(t *testing.T) {
	for _, body := range []string{
		"channel,model,input_cost,output_cost\na,ok,1,2\nb,bad,-0.00000000001,2\n",
		"channel,model,input_cost,output_cost\na,ok,1,2\nb,bad,NaN,2\n",
		"channel,model,input_cost,output_cost\na,ok,1,2\nb,bad,1000000001,2\n",
		"channel,model,input_cost,output_cost\n",
		"a,ok,1,2\n" + strings.Repeat(" ", supplierCSVMaxBytes),
	} {
		h, _ := newCostTestHandler(t)
		w := invokeCost(t, h.ImportSupplierCosts, "POST", "/import", body, nil)
		rows, _ := h.repo.ListSupplierCosts()
		if w.Code != http.StatusBadRequest || len(rows) != 0 {
			t.Fatalf("invalid import: status=%d rows=%d", w.Code, len(rows))
		}
	}
}

func TestEcosystemAPIsRejectIDsUnsafeForDownloadAndClientConfig(t *testing.T) {
	h, _ := newCostTestHandler(t)
	for _, id := range []string{"../escape", "a/b", "bad;command", "bad\"name", "bad\nname", "."} {
		body := fmt.Sprintf(`{"id":%q,"name":"Test","description":"Test","endpoint":"npx test"}`, id)
		for _, handler := range []func(){
			func() {
				w := invokeCost(t, h.SaveSkill, "POST", "/skills", body, nil)
				if w.Code != 400 {
					t.Fatalf("skill %q: %d", id, w.Code)
				}
			},
			func() {
				w := invokeCost(t, h.SaveMCPServer, "POST", "/mcp", body, nil)
				if w.Code != 400 {
					t.Fatalf("mcp %q: %d", id, w.Code)
				}
			},
		} {
			handler()
		}
	}
}
