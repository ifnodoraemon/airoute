package billing

import "strings"

// EstimateFixedCostCNY reserves every billable unit without assuming an off-peak
// discount. Token-based calls can also have a per-request fixed component.
func EstimateFixedCostCNY(model, group string, units int) float64 {
	if GlobalEngine == nil {
		return 0
	}
	price := GlobalEngine.GetPriceWithGroup(model, group)
	if price == nil || price.FixedPrice <= 0 {
		return 0
	}
	cost := price.FixedPrice * float64(max(1, units))
	grp := strings.ToLower(strings.TrimSpace(group))
	if grp != "" && grp != "default" && strings.ToLower(strings.TrimSpace(price.GroupName)) != grp {
		cost *= GlobalEngine.GetGroupMultiplier(grp)
	}
	return roundPrecision(cost)
}
