package storage

func correlateSupplierChannels(ordered []*SupplierReconciliationItem, supplierMap map[string]*SupplierReconciliationItem, channels []*ChannelRecord, filter string) []*SupplierReconciliationItem {
	byName := make(map[string]*ChannelRecord)
	for _, ch := range channels {
		byName[ch.Name] = ch
	}
	for _, supplier := range ordered {
		if ch, ok := byName[supplier.ChannelName]; ok {
			applySupplierChannel(supplier, ch)
		} else {
			supplier.ChannelStatus, supplier.ChannelType = "deleted", "external"
		}
	}
	if filter == "" {
		for _, ch := range channels {
			if _, exists := supplierMap[ch.Name]; !exists {
				ordered = append(ordered, emptySupplierChannel(ch))
			}
		}
	} else if len(ordered) == 0 {
		for _, ch := range channels {
			if ch.Name == filter {
				ordered = append(ordered, emptySupplierChannel(ch))
				break
			}
		}
	}
	if ordered == nil {
		ordered = make([]*SupplierReconciliationItem, 0)
	}
	return ordered
}

func applySupplierChannel(supplier *SupplierReconciliationItem, ch *ChannelRecord) {
	supplier.ChannelID = ch.ID
	supplier.ChannelType = string(ch.Type)
	supplier.ChannelStatus = ch.Status
	supplier.BaseURL = ch.BaseURL
	supplier.Priority = ch.Priority
	supplier.Weight = ch.Weight
}

func emptySupplierChannel(ch *ChannelRecord) *SupplierReconciliationItem {
	supplier := &SupplierReconciliationItem{
		ChannelName: ch.Name, SuccessRate: 100.0,
		Models: make([]*SupplierModelReconciliation, 0),
	}
	applySupplierChannel(supplier, ch)
	return supplier
}
