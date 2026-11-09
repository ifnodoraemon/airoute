package storage

import (
	"fmt"
)

// GetSupplierReconciliation aggregates usage logs by supplier/channel with full token and cost reconciliation.
func (r *Repository) GetSupplierReconciliation(f ChannelReconciliationFilter) (*SupplierReconciliationReport, error) {
	whereClause, args := r.supplierReconciliationFilter(f)
	rows, err := r.db.Query(supplierChannelQuery(whereClause), args...)
	if err != nil {
		return nil, fmt.Errorf("query supplier reconciliation error: %w", err)
	}
	defer rows.Close()
	orderedSuppliers, supplierMap, totals, err := readSupplierChannels(rows)
	if err != nil {
		return nil, err
	}
	mRows, err := r.db.Query(supplierModelQuery(whereClause), args...)
	if err == nil {
		defer mRows.Close()
		readSupplierModels(mRows, supplierMap)
	}
	allChannels, _ := r.ListChannels()
	orderedSuppliers = correlateSupplierChannels(orderedSuppliers, supplierMap, allChannels, f.Channel)
	totalCount := len(allChannels)
	if len(orderedSuppliers) > totalCount {
		totalCount = len(orderedSuppliers)
	}
	report := &SupplierReconciliationReport{
		Summary:   supplierReconciliationSummary(totals, len(supplierMap), totalCount),
		Suppliers: orderedSuppliers,
	}
	report.TimeRange.StartTime = f.StartTime
	report.TimeRange.EndTime = f.EndTime
	return report, nil
}
